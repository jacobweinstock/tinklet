package kube

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestCreateJob(t *testing.T) {
	var parallelism int32 = 1
	tests := map[string]struct {
		clientset   *fake.Clientset
		expectedJob *batchv1.Job
		expectedErr error
	}{
		"success": {clientset: fake.NewSimpleClientset(&batchv1.Job{
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{},
			Spec:       batchv1.JobSpec{},
			Status:     batchv1.JobStatus{},
		}), expectedJob: &batchv1.Job{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test_job",
				Namespace: "test_ns",
				Labels:    map[string]string{},
			},
			Spec: batchv1.JobSpec{
				Parallelism:  &parallelism,
				BackoffLimit: new(int32),
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{},
					Spec: v1.PodSpec{
						Containers:       []v1.Container{{Name: "test_job", Image: "alpine", Command: []string{"/bin/sh", "sleep", "2"}}},
						RestartPolicy:    "Never",
						ImagePullSecrets: []v1.LocalObjectReference{{}},
					},
				},
			},
			Status: batchv1.JobStatus{},
		}},
		"failure": {clientset: fake.NewSimpleClientset(), expectedErr: errors.New("failed to create K8s job: error creating job")},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.expectedErr != nil {
				tc.clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, &batchv1.Job{}, errors.New("error creating job")
				})
			}
			client := Client{Conn: tc.clientset}
			job, err := client.createJob(context.Background(), "test_ns", "test_job", "alpine", []string{"/bin/sh", "sleep", "2"}, map[string]string{}, "", true)
			if err != nil {
				if tc.expectedErr != nil {
					if diff := cmp.Diff(err.Error(), tc.expectedErr.Error()); diff != "" {
						t.Fatal(diff)
					}
				} else {
					t.Fatal(err)
				}
			} else {
				if diff := cmp.Diff(job, tc.expectedJob); diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}

/*
func connectToK8s() kubernetes.Interface {
	home, exists := os.LookupEnv("HOME")
	if !exists {
		home = "/root"
	}

	configPath := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		log.Panicln("failed to create K8s config")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Panicln("Failed to create K8s clientset")
	}

	return clientset
}

func TestLive(t *testing.T) {
	t.Skip()
	client := Client{Conn: connectToK8s()}
	job, err := client.CreateJob(context.Background(), "default", "testing", "alpine", []string{"/bin/sleep", "5"}, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal(job)
}


func TestJobExecComplete(t *testing.T) {
	t.Skip()
	client := Client{Conn: connectToK8s()}
	complete, state, err := client.JobExecComplete(context.Background(), "default", "testing")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(complete)
	t.Fatalf("%+v", state)
}
*/
