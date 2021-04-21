package kube

import (
	"context"

	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Client for communicating with kubernetes
type Client struct {
	Conn kubernetes.Interface
}

// CreateImagePullSecret from a base64 encoded auth string
func (c Client) CreateImagePullSecret(ctx context.Context, namespace string, secretName string, authString string) error {
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

// CreateJob creates a kubernetes job
func (c Client) CreateJob(ctx context.Context, namespace string, jobName string, image string, cmd []string, imagePullSecretName string) (*batchv1.Job, error) {
	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
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

// JobExecComplete will determine if the job has completed or not regardless of completion state or status
// given the jobs name, look up the completion status of the backing pod
func (c *Client) JobExecComplete(ctx context.Context, namespace string, jobName string) (complete bool, state v1.ContainerState, err error) {
	pods := c.Conn.CoreV1().Pods(namespace)
	pod, err := pods.List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
	if err != nil {
		return false, v1.ContainerState{}, errors.Wrap(err, "error getting pod")
	}
	if len(pod.Items) == 0 {
		return false, v1.ContainerState{}, errors.New("job not found")
	}
	for _, elem := range pod.Items {
		st := elem.Status.ContainerStatuses[0].State
		if st.Terminated == nil {
			return false, v1.ContainerState{}, nil
		}
		return true, st, nil
	}
	return false, v1.ContainerState{}, nil
}
