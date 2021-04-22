package kube

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/workflow"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
)

// Client for communicating with kubernetes
type Client struct {
	Conn                     kubernetes.Interface
	RegistryAuth             map[string]string
	action                   *workflow.WorkflowAction
	taskNamespace            string
	taskNamespaceIsEphemeral bool
	taskPullSecret           string
	jobSpec                  *batchv1.Job
	workflowID               string
}

func (c *Client) SetActionData(ctx context.Context, workflowID string, action *workflow.WorkflowAction) {
	c.action = action
	c.workflowID = workflowID
}

func getRegistryAuth(regAuth map[string]string, imageName string) string {
	for reg, auth := range regAuth {
		if strings.HasPrefix(imageName, reg) {
			return auth
		}
	}
	return ""
}

type constrainedString string

func (a constrainedString) maxChars(max int) string {
	if len(a) > max {
		v := a[:max]
		return string(v)
	}
	return string(a)
}

func (c *Client) PrepareEnv(ctx context.Context, id string) error {
	var multiErr error
	// 1. create namespace; once per task
	// check if we can create namespaces, if we can we run workflow tasks in ephemeral namespaces
	// if we cant create namespaces, the tinklet namespace must exist
	if c.taskNamespace == "" {
		// kubernetes namespace max length is 63 characters
		nmsp := constrainedString(fmt.Sprintf("tinklet-%v-%v", id, time.Now().UnixNano()))
		ns := nmsp.maxChars(63)
		// check if the namespace exists
		_, err := c.Conn.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		if err == nil {
			c.taskNamespace = ns
			c.taskNamespaceIsEphemeral = true
			return nil
		} else {
			multiErr = multierror.Append(multiErr, err)
		}
		_, err = c.Conn.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		}, metav1.CreateOptions{})
		if err == nil {
			c.taskNamespace = ns
			c.taskNamespaceIsEphemeral = true
			return nil
		} else {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	// check if the tinklet namespace exists
	// TODO: if we cant create namespaces, make the namespace to use user-define-able
	_, err := c.Conn.CoreV1().Namespaces().Get(ctx, "tinklet", metav1.GetOptions{})
	if err != nil {
		return errors.WithMessage(multierror.Append(multiErr, err), "tinklet namespace not found")
	}

	c.taskNamespace = "tinklet"
	return nil
}

func (c *Client) CleanEnv(ctx context.Context) error {
	// 3. delete namespace
	if c.taskNamespaceIsEphemeral {
		defer func() { c.taskNamespace = "" }()
		// no grace period; delete now
		var gracePeriod int64 = 0
		// delete all descendends in the foreground
		policy := metav1.DeletePropagationForeground
		return c.Conn.CoreV1().Namespaces().Delete(ctx, c.taskNamespace, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &policy})
	}

	return nil
}

func (c *Client) Prepare(ctx context.Context, imageName string) (id string, err error) {
	// 2. create pull secrets; once per task
	if c.taskPullSecret == "" {
		regAuth := getRegistryAuth(c.RegistryAuth, imageName)
		if regAuth != "" {
			name := "regcred"
			if _, err := c.Conn.CoreV1().Secrets(c.taskNamespace).Create(ctx, toDockerConfigSecret(name, regAuth), metav1.CreateOptions{}); err != nil {
				return "", err
			}
			c.taskPullSecret = name
		}
	}
	// 3. create regular secrets
	// 4. create job without starting it (https://cloud.google.com/kubernetes-engine/docs/how-to/jobs#managing_parallelism)
	labels := map[string]string{}
	job, err := c.createJob(ctx, c.taskNamespace, strings.ReplaceAll(c.action.Name, " ", "-"), c.action.Image, c.action.Command, labels, c.taskPullSecret, false)
	if err != nil {
		return "", err
	}
	c.jobSpec = job

	// TODO: wait for the k8s resource to be stable before returning?

	return job.Name, nil
}

func (c *Client) Destroy(ctx context.Context) error {
	// no grace period; delete now
	var gracePeriod int64 = 0
	// delete all descendends in the foreground
	policy := metav1.DeletePropagationForeground
	deleteOpts := metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &policy}
	var errs error
	// 1. delete secrets
	if c.taskPullSecret != "" {
		if err := c.Conn.CoreV1().Secrets(c.taskNamespace).Delete(ctx, c.taskPullSecret, deleteOpts); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	c.taskPullSecret = ""
	// 2. delete job
	if c.jobSpec != nil {
		if err := c.Conn.BatchV1().Jobs(c.taskNamespace).Delete(ctx, c.jobSpec.Name, deleteOpts); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	c.jobSpec = nil
	return errs
}

// Run the action; wait for complete
func (c *Client) Run(ctx context.Context, id string) error {
	// 1. start the job; set parallelism to 1
	spec := *c.jobSpec
	var parallelism int32 = 1
	spec.Spec.Parallelism = &parallelism
	if _, err := c.Conn.BatchV1().Jobs(c.taskNamespace).Update(ctx, c.jobSpec, metav1.UpdateOptions{}); err != nil {
		if _, err := c.Conn.BatchV1().Jobs(c.taskNamespace).Update(ctx, c.jobSpec, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}

	// 2. wait for job/pod completion
	return c.waitFor(ctx, func(e watch.Event) (bool, error) {
		switch t := e.Type; t {
		case watch.Added, watch.Modified:
			pod, ok := e.Object.(*v1.Pod)
			if !ok {
				return false, nil
			}
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Terminated != nil {
					return true, nil
				}
			}
		}
		return false, nil
	})

}

func (c *Client) waitFor(ctx context.Context, conditionFunc watchtools.ConditionFunc) error {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.action.Timeout)*time.Second)
	defer cancel()
	label := fmt.Sprintf("job-name=%s", c.jobSpec.Name)
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return c.Conn.CoreV1().Pods(c.taskNamespace).List(ctx, metav1.ListOptions{
				LabelSelector: label,
			})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return c.Conn.CoreV1().Pods(c.taskNamespace).Watch(ctx, metav1.ListOptions{
				LabelSelector: label,
			})
		},
	}

	_, err := watchtools.UntilWithSync(ctx, lw, &v1.Pod{}, nil, conditionFunc)
	return err
}

/*
// createImagePullSecret from a base64 encoded auth string
func (c *Client) createImagePullSecret(ctx context.Context, namespace string, secretName string, authString string) error {
	secretSpec := &v1.Secret{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{".dockerconfigjson": []byte(authString)},
		Type: v1.SecretTypeDockerConfigJson,
	}
	secrets := c.Conn.CoreV1().Secrets(namespace)
	_, err := secrets.Create(ctx, secretSpec, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}
*/

// createJob creates a kubernetes job
func (c *Client) createJob(ctx context.Context, namespace string, jobName string, image string, cmd []string, labels map[string]string, imagePullSecretName string, startImmediately bool) (*batchv1.Job, error) {
	var parallelism *int32
	if startImmediately {
		var n int32 = 1
		parallelism = &n
	}
	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Parallelism: parallelism,
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    jobName,
							Image:   image,
							Command: cmd,
						},
					},
					RestartPolicy: v1.RestartPolicyNever,
					ImagePullSecrets: []v1.LocalObjectReference{
						{
							Name: imagePullSecretName,
						},
					},
				},
			},
			BackoffLimit: new(int32),
		},
	}
	jobs := c.Conn.BatchV1().Jobs(namespace)
	job, err := jobs.Create(ctx, jobSpec, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create K8s job")
	}
	return job, nil
}
