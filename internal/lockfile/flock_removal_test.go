package lockfile_test

import "testing"

func TestFlockRemovedAfterDaemonMigration(t *testing.T) {
	t.Skip("TODO(daemon-stable): Unskip this test when ready to remove flock. " +
		"If this test fails, it means flock references still exist and should be cleaned up.")
}
