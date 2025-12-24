package inspector

import (
	"os"
	"testing"
	"time"

	"github.com/cavaliergopher/rpm"

	"github.com/e2llm/rpmrepo-update/pkg/metadata"
)

func TestDepFlagsToString(t *testing.T) {
	tests := []struct {
		flags   int
		want    string
		wantPre bool
	}{
		{0, "", false},
		{rpm.DepFlagEqual, "EQ", false},
		{rpm.DepFlagLesser, "LT", false},
		{rpm.DepFlagGreater, "GT", false},
		{rpm.DepFlagLesserOrEqual, "LE", false},
		{rpm.DepFlagGreaterOrEqual, "GE", false},
		{rpm.DepFlagPrereq, "", true},
		{rpm.DepFlagEqual | rpm.DepFlagPrereq, "EQ", true},
	}

	for _, tt := range tests {
		got, pre := depFlagsToString(tt.flags)
		if got != tt.want {
			t.Errorf("depFlagsToString(%d) = %q, want %q", tt.flags, got, tt.want)
		}
		if pre != tt.wantPre {
			t.Errorf("depFlagsToString(%d) pre = %v, want %v", tt.flags, pre, tt.wantPre)
		}
	}
}

func TestMinLen(t *testing.T) {
	tests := []struct {
		a, b, c int
		want    int
	}{
		{1, 2, 3, 1},
		{3, 2, 1, 1},
		{2, 1, 3, 1},
		{5, 5, 5, 5},
		{0, 10, 20, 0},
		{100, 50, 75, 50},
	}

	for _, tt := range tests {
		got := minLen(tt.a, tt.b, tt.c)
		if got != tt.want {
			t.Errorf("minLen(%d, %d, %d) = %d, want %d", tt.a, tt.b, tt.c, got, tt.want)
		}
	}
}

func TestFilesFromRPMEmpty(t *testing.T) {
	files := filesFromRPM(nil)
	if files != nil {
		t.Errorf("filesFromRPM(nil) = %v, want nil", files)
	}
}

func TestDepsFromRPMEmpty(t *testing.T) {
	deps := depsFromRPM(nil)
	if deps != nil {
		t.Errorf("depsFromRPM(nil) = %v, want nil", deps)
	}
}

func TestDepsFromRPM(t *testing.T) {
	// Test with empty slice
	deps := depsFromRPM([]rpm.Dependency{})
	if len(deps) != 0 {
		t.Errorf("depsFromRPM([]) = %v, want empty slice", deps)
	}
}

func TestFilesFromRPM(t *testing.T) {
	// Test with empty slice
	files := filesFromRPM([]rpm.FileInfo{})
	if len(files) != 0 {
		t.Errorf("filesFromRPM([]) = %v, want empty slice", files)
	}
}

// TestInspectRPMInvalidData tests that InspectRPM returns an error for invalid data
func TestInspectRPMInvalidData(t *testing.T) {
	invalidData := []byte("not a valid RPM file")

	// Create a mock FileInfo
	mockInfo := mockFileInfo{size: int64(len(invalidData))}

	_, err := InspectRPM("test.rpm", invalidData, mockInfo, "sha256", "test.rpm")
	if err == nil {
		t.Error("InspectRPM should return error for invalid RPM data")
	}
}

// TestInspectRPMEmptyData tests that InspectRPM returns an error for empty data
func TestInspectRPMEmptyData(t *testing.T) {
	mockInfo := mockFileInfo{size: 0}

	_, err := InspectRPM("test.rpm", []byte{}, mockInfo, "sha256", "test.rpm")
	if err == nil {
		t.Error("InspectRPM should return error for empty RPM data")
	}
}

// mockFileInfo implements fs.FileInfo for testing
type mockFileInfo struct {
	size int64
}

func (m mockFileInfo) Name() string       { return "test.rpm" }
func (m mockFileInfo) Size() int64        { return m.size }
func (m mockFileInfo) Mode() os.FileMode  { return 0o644 }
func (m mockFileInfo) ModTime() time.Time { return time.Now() }
func (m mockFileInfo) IsDir() bool        { return false }
func (m mockFileInfo) Sys() interface{}   { return nil }

// Ensure Package type is correctly used (compile-time check)
var _ metadata.Package
