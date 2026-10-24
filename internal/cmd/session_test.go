package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionCleanHandoffAndInterruptionRecovery(t *testing.T) {
	tempDir := t.TempDir()
	testDBPath := filepath.Join(tempDir, "session_test.db")

	runCmd := func(args ...string) (string, error) {
		buf := new(bytes.Buffer)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)

		fullArgs := append([]string{"--db", testDBPath, "--json"}, args...)
		processed := PreprocessArgs(fullArgs)
		RootCmd.SetArgs(processed)
		err := RootCmd.Execute()
		return buf.String(), err
	}

	// ----------------------------------------------------
	// STEP 1: First agent starts session cleanly
	// ----------------------------------------------------
	out, err := runCmd("init")
	if err != nil {
		t.Fatalf("first tm init failed: %v, out: %s", err, out)
	}

	var init1 InitResponse
	if err := json.Unmarshal([]byte(out), &init1); err != nil {
		t.Fatalf("failed to parse init1 JSON: %v", err)
	}
	if init1.ResumedFromInterrupt {
		t.Errorf("expected fresh session, but marked as resumed from interrupt")
	}
	session1ID := init1.SessionID

	// Agent creates and starts Task 1
	out, err = runCmd("add", "Implement auth boundary", "priority:H")
	if err != nil {
		t.Fatalf("tm add failed: %v", err)
	}
	out, err = runCmd("1", "start")
	if err != nil {
		t.Fatalf("tm 1 start failed: %v", err)
	}
	out, err = runCmd("1", "done")
	if err != nil {
		t.Fatalf("tm 1 done failed: %v", err)
	}

	// Agent performs clean handoff
	out, err = runCmd("handoff", "Finished auth boundary implementation", "next:implement-tui")
	if err != nil {
		t.Fatalf("tm handoff failed: %v", err)
	}
	var handoffRes HandoffResponse
	if err := json.Unmarshal([]byte(out), &handoffRes); err != nil {
		t.Fatalf("failed to parse handoff JSON: %v", err)
	}
	if handoffRes.SessionID != session1ID {
		t.Errorf("expected handoff session %s, got %s", session1ID, handoffRes.SessionID)
	}

	// ----------------------------------------------------
	// STEP 2: Second agent starts session after clean handoff
	// ----------------------------------------------------
	out, err = runCmd("init")
	if err != nil {
		t.Fatalf("second tm init failed: %v", err)
	}
	var init2 InitResponse
	if err := json.Unmarshal([]byte(out), &init2); err != nil {
		t.Fatalf("failed to parse init2 JSON: %v", err)
	}
	if init2.ResumedFromInterrupt {
		t.Errorf("expected clean handoff, but got resumed from interrupt")
	}
	if init2.LastHandoff != "Finished auth boundary implementation" {
		t.Errorf("unexpected previous summary: %s", init2.LastHandoff)
	}

	// ----------------------------------------------------
	// STEP 3: Agent 2 works on Task 2, then suddenly crashes
	// ----------------------------------------------------
	out, err = runCmd("add", "Implement Bubbletea TUI", "priority:C")
	if err != nil {
		t.Fatalf("tm add task 2 failed: %v", err)
	}
	out, err = runCmd("2", "start")
	if err != nil {
		t.Fatalf("tm 2 start failed: %v", err)
	}
	out, err = runCmd("2", "log", "Constructed initial Bubbletea model with task list")
	if err != nil {
		t.Fatalf("tm 2 log failed: %v", err)
	}

	// Crash: no tm handoff called!

	// ----------------------------------------------------
	// STEP 4: Third agent restarts after crash: tm init
	// ----------------------------------------------------
	out, err = runCmd("init")
	if err != nil {
		t.Fatalf("third tm init failed: %v", err)
	}
	var init3 InitResponse
	if err := json.Unmarshal([]byte(out), &init3); err != nil {
		t.Fatalf("failed to parse init3 JSON: %v", err)
	}

	if !init3.ResumedFromInterrupt {
		t.Errorf("expected ResumedFromInterrupt=true after sudden crash")
	}
	if init3.ActiveTask == nil || init3.ActiveTask.ID != 2 {
		t.Errorf("expected active task #2 restored, got %v", init3.ActiveTask)
	}
	if !strings.Contains(init3.LastHandoff, "Task #2 note: Constructed initial Bubbletea model") {
		t.Errorf("expected last checkpoint note in LastHandoff, got: %s", init3.LastHandoff)
	}
	if len(init3.RecentObservations) == 0 {
		t.Errorf("expected recent observations in init3")
	}
}
