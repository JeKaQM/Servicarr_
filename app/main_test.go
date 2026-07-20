package main

import "testing"

func TestCheckResultIsConfirmed(t *testing.T) {
	tests := []struct {
		name                string
		checkOK             bool
		consecutiveFailures int
		want                bool
	}{
		{name: "successful check", checkOK: true, want: true},
		{name: "first failure is debounced", consecutiveFailures: 1, want: false},
		{name: "second failure is confirmed", consecutiveFailures: 2, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := checkResultIsConfirmed(test.checkOK, test.consecutiveFailures); got != test.want {
				t.Fatalf("checkResultIsConfirmed(%t, %d) = %t, want %t", test.checkOK, test.consecutiveFailures, got, test.want)
			}
		})
	}
}
