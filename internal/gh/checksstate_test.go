package gh

import (
	"encoding/json"
	"testing"
)

func TestChecksStateJudgesLikeTheChecksTab(t *testing.T) {
	cases := []struct {
		name, rollup, want string
	}{
		{"a cancelled run beside passing ones", `"state": "FAILURE", "contexts": {"checkRunCountsByState": [{"state": "CANCELLED", "count": 1}, {"state": "SUCCESS", "count": 11}],
			"statusContextCountsByState": [{"state": "SUCCESS", "count": 2}]}`, "SUCCESS"},
		{"a failure outranks a running check", `"state": "PENDING", "contexts": {"checkRunCountsByState": [{"state": "IN_PROGRESS", "count": 1}, {"state": "TIMED_OUT", "count": 1}]}`, "FAILURE"},
		{"a pending status", `"state": "PENDING", "contexts": {"checkRunCountsByState": [{"state": "SUCCESS", "count": 1}], "statusContextCountsByState": [{"state": "PENDING", "count": 1}]}`, "PENDING"},
		{"no counts falls back to GitHub", `"state": "ERROR", "contexts": {}`, "ERROR"},
	}
	for _, each := range cases {
		var commits lastCommit
		if err := json.Unmarshal([]byte(`{"nodes": [{"commit": {"statusCheckRollup": {`+each.rollup+`}}}]}`), &commits); err != nil {
			t.Fatal(err)
		}
		if got := commits.checksState(); got != each.want {
			t.Errorf("%s: %s, want %s", each.name, got, each.want)
		}
	}
	if (lastCommit{}).checksState() != "" {
		t.Error("a commit without checks has no state")
	}
}
