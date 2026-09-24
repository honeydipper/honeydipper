package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestShouldStartImmediatelyDefaultsTrue(t *testing.T) {
	assert.True(t, shouldStartImmediately(map[string]interface{}{}))
	assert.True(t, shouldStartImmediately(nil))
	assert.False(t, shouldStartImmediately(map[string]interface{}{"start_immediately": false}))
	assert.True(t, shouldStartImmediately(map[string]interface{}{"start_immediately": true}))
}

func TestGetExistingJobReturnsSuspendedJob(t *testing.T) {
	suspended := true
	client := fake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-suspended",
			Namespace: DefaultNamespace,
			Labels:    map[string]string{LabelHoneydipperUniqueIdentifier: "abc123"},
		},
		Spec: batchv1.JobSpec{Suspend: &suspended},
	})

	job := getExistingJob(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{LabelHoneydipperUniqueIdentifier: "abc123"}},
	}, client.BatchV1().Jobs(DefaultNamespace))

	if assert.NotNil(t, job) {
		assert.Equal(t, "job-suspended", job.Name)
		assert.True(t, isJobSuspended(job))
	}
}

func TestGetExistingJobSkipsCompletedJobs(t *testing.T) {
	client := fake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-complete",
			Namespace: DefaultNamespace,
			Labels:    map[string]string{LabelHoneydipperUniqueIdentifier: "abc123"},
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: "True"}},
		},
	})

	job := getExistingJob(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{LabelHoneydipperUniqueIdentifier: "abc123"}},
	}, client.BatchV1().Jobs(DefaultNamespace))

	assert.Nil(t, job)
}

func TestStartExistingJobUnsuspendsJob(t *testing.T) {
	suspended := true
	client := fake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job-suspended", Namespace: DefaultNamespace},
		Spec:       batchv1.JobSpec{Suspend: &suspended},
	})

	job, err := startExistingJob(context.Background(), client.BatchV1().Jobs(DefaultNamespace), "job-suspended")
	if assert.NoError(t, err) && assert.NotNil(t, job) {
		assert.False(t, isJobSuspended(job))
		assert.True(t, job.Spec.Suspend != nil)
	}
	stored, err := client.BatchV1().Jobs(DefaultNamespace).Get(context.Background(), "job-suspended", metav1.GetOptions{})
	if assert.NoError(t, err) {
		assert.False(t, isJobSuspended(stored))
	}
}

func TestStartExistingJobIsIdempotent(t *testing.T) {
	client := fake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job-running", Namespace: DefaultNamespace},
	})

	job, err := startExistingJob(context.Background(), client.BatchV1().Jobs(DefaultNamespace), "job-running")
	if assert.NoError(t, err) && assert.NotNil(t, job) {
		assert.False(t, isJobSuspended(job))
	}
}

func TestGetJobState(t *testing.T) {
	suspended := true
	assert.Equal(t, "suspended", getJobState(&batchv1.Job{Spec: batchv1.JobSpec{Suspend: &suspended}}))
	assert.Equal(t, "running", getJobState(&batchv1.Job{Status: batchv1.JobStatus{Active: 1}}))
	assert.Equal(t, StatusPending, getJobState(&batchv1.Job{}))
	assert.Equal(t, "completed", getJobState(&batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: "True"}}}}))
	assert.Equal(t, "failed", getJobState(&batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: "True"}}}}))
}
