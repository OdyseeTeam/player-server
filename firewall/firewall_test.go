package firewall

import (
	"strconv"
	"testing"
	"time"
)

func TestCheckIPAccess(t *testing.T) {
	ip := "192.168.0.1"
	endpoint := "/api/v1/example"

	// Test the first five accesses for an IP don't exceed the limit
	for i := 1; i <= 5; i++ {
		result, _ := IsIpAbusingResources(ip, endpoint+strconv.Itoa(i))
		if result {
			t.Errorf("Expected result to be false, got %v for endpoint %s", result, endpoint+strconv.Itoa(i))
		}
	}

	// Test the sixth access for an IP exceeds the limit
	result, _ := IsIpAbusingResources(ip, endpoint+"6")
	if !result {
		t.Errorf("Expected result to be true, got %v for endpoint %s", result, endpoint+"6")
	}

	// Wait for the window size to elapse
	time.Sleep(WindowSize)

	// Test the access for an IP after the window size elapses doesn't exceed the limit
	result, _ = IsIpAbusingResources(ip, endpoint+"7")
	if result {
		t.Errorf("Expected result to be false, got %v for endpoint %s", result, endpoint+"7")
	}

}
