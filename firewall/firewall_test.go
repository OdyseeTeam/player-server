package firewall

import (
	"strconv"
	"testing"
)

func TestCheckIPAccess(t *testing.T) {
	ip := "192.168.1.1"
	for i := 0; i < MAX_STRINGS_PER_IP; i++ {
		if IsIpAbusingResources(ip, "string "+strconv.Itoa(i)) {
			t.Errorf("Expected false, but got true")
		}
	}
	if !IsIpAbusingResources(ip, "string "+strconv.Itoa(MAX_STRINGS_PER_IP)) {
		t.Errorf("Expected true, but got false")
	}
}
