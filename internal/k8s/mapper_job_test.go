package k8s

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMapJobStatus(t *testing.T) {
	completions := func(n int32) *int32 { return &n }
	suspend := func(b bool) *bool { return &b }
	failed := batchv1.JobCondition{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}
	complete := batchv1.JobCondition{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}

	cases := []struct {
		name string
		spec batchv1.JobSpec
		stat batchv1.JobStatus
		want string
	}{
		{
			name: "retrying job with failed pods but still active stays active",
			spec: batchv1.JobSpec{Completions: completions(1)},
			stat: batchv1.JobStatus{Active: 1, Failed: 2},
			want: "active",
		},
		{
			name: "terminal JobFailed condition is failed",
			spec: batchv1.JobSpec{Completions: completions(1)},
			stat: batchv1.JobStatus{Failed: 3, Conditions: []batchv1.JobCondition{failed}},
			want: "failed",
		},
		{
			name: "JobComplete condition is completed",
			spec: batchv1.JobSpec{Completions: completions(1)},
			stat: batchv1.JobStatus{Succeeded: 1, Conditions: []batchv1.JobCondition{complete}},
			want: "completed",
		},
		{
			name: "succeeded count reaching completions is completed",
			spec: batchv1.JobSpec{Completions: completions(2)},
			stat: batchv1.JobStatus{Succeeded: 2},
			want: "completed",
		},
		{
			name: "suspended wins over everything",
			spec: batchv1.JobSpec{Completions: completions(1), Suspend: suspend(true)},
			stat: batchv1.JobStatus{Failed: 1},
			want: "suspended",
		},
		{
			name: "exhausted job with failures and no active pods is failed",
			spec: batchv1.JobSpec{Completions: completions(1)},
			stat: batchv1.JobStatus{Failed: 1},
			want: "failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "j", Namespace: "default"},
				Spec:       tc.spec,
				Status:     tc.stat,
			}
			if got := MapJob(j).Status; got != tc.want {
				t.Fatalf("MapJob status = %q, want %q", got, tc.want)
			}
		})
	}
}
