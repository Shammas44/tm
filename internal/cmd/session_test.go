package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"taskmanager/internal/model"
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
	if m, ok := init2.LastHandoff.(map[string]any); ok {
		if m["summary"] != "Finished auth boundary implementation" {
			t.Errorf("unexpected previous summary in handoff map: %v", m)
		}
	} else if init2.LastHandoff != "Finished auth boundary implementation" {
		t.Errorf("unexpected previous summary: %v", init2.LastHandoff)
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
	handoffStr := fmt.Sprint(init3.LastHandoff)
	if !strings.Contains(handoffStr, "Task #2 note: Constructed initial Bubbletea model") &&
		!strings.Contains(init3.LastCheckpoint, "Task #2 note: Constructed initial Bubbletea model") {
		t.Errorf("expected last checkpoint note in LastHandoff or LastCheckpoint, got handoff: %s, checkpoint: %s", handoffStr, init3.LastCheckpoint)
	}
	if len(init3.RecentObservations) == 0 {
		t.Errorf("expected recent observations in init3")
	}
}

func TestSessionInspectionAndStructuredHandoff(t *testing.T) {
	tempDir := t.TempDir()
	testDBPath := filepath.Join(tempDir, "session_inspect.db")

	runCmd := func(args ...string) (string, error) {
		jsonFlag = false
		quietFlag = false
		var buf bytes.Buffer
		fullArgs := append([]string{"--db", testDBPath}, args...)
		RootCmd.SetOut(&buf)
		RootCmd.SetErr(&buf)
		RootCmd.SetArgs(PreprocessArgs(fullArgs))
		err := RootCmd.Execute()
		return buf.String(), err
	}

	// 1. Initialize session with custom agent
	out, err := runCmd("init", "--agent", "agent-alpha", "--json")
	if err != nil {
		t.Fatalf("init failed: %v, out: %s", err, out)
	}
	var initResp InitResponse
	if err := json.Unmarshal([]byte(out), &initResp); err != nil {
		t.Fatalf("failed to parse init JSON: %v", err)
	}
	sessionID := initResp.SessionID

	// 2. Add and log on task
	_, _ = runCmd("add", "Refactor core", "priority:H")
	_, _ = runCmd("1", "start")
	_, _ = runCmd("1", "log", "Identified memory leak in slab allocator")

	// 3. Record structured JSON handoff
	jsonHandoff := `{"summary": "Phase 1 complete", "files_modified": ["slab.c", "pool.c"], "status": "green"}`
	out, err = runCmd("handoff", jsonHandoff, "--json")
	if err != nil {
		t.Fatalf("handoff failed: %v, out: %s", err, out)
	}
	var handoffResp HandoffResponse
	if err := json.Unmarshal([]byte(out), &handoffResp); err != nil {
		t.Fatalf("failed to parse handoff JSON: %v", err)
	}
	hMap, ok := handoffResp.Handoff.(map[string]any)
	if !ok || hMap["summary"] != "Phase 1 complete" {
		t.Errorf("expected parsed handoff map with summary, got: %+v", handoffResp.Handoff)
	}

	// 4. List sessions (terminal table and JSON)
	out, err = runCmd("session", "list")
	if err != nil {
		t.Fatalf("session list failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, sessionID[:8]) || !strings.Contains(out, "agent-alpha") {
		t.Errorf("expected session short ID and agent in table, got:\n%s", out)
	}

	out, err = runCmd("session", "--json")
	if err != nil {
		t.Fatalf("session --json failed: %v, out: %s", err, out)
	}
	var sessionsList []*model.Session
	if err := json.Unmarshal([]byte(out), &sessionsList); err != nil {
		t.Fatalf("failed to parse session list JSON: %v", err)
	}
	if len(sessionsList) != 1 || sessionsList[0].ID != sessionID {
		t.Errorf("expected 1 session matching ID %s, got: %+v", sessionID, sessionsList)
	}

	// 5. Show session by short prefix
	shortID := sessionID[:8]
	out, err = runCmd("session", shortID)
	if err != nil {
		t.Fatalf("session show by short prefix failed: %v, out: %s", err, out)
	}
	if !strings.Contains(out, "Session: "+sessionID) || !strings.Contains(out, "Phase 1 complete") {
		t.Errorf("expected session details with handoff, got:\n%s", out)
	}

	// 6. Show session details JSON
	out, err = runCmd("session", "show", sessionID, "--json")
	if err != nil {
		t.Fatalf("session show JSON failed: %v, out: %s", err, out)
	}
	var detail SessionDetailResponse
	if err := json.Unmarshal([]byte(out), &detail); err != nil {
		t.Fatalf("failed to parse session detail JSON: %v", err)
	}
	if detail.Session.ID != sessionID || len(detail.Observations) != 1 {
		t.Errorf("expected session ID %s with 1 observation, got: %+v", sessionID, detail)
	}
	if len(detail.TasksTouched) != 1 || detail.TasksTouched[0] != 1 {
		t.Errorf("expected task #1 touched, got: %v", detail.TasksTouched)
	}

	// 7. Verify Turn-1 consumption in subsequent tm init receives structured handoff
	out, err = runCmd("init", "--agent", "agent-beta", "--json")
	if err != nil {
		t.Fatalf("init 2 failed: %v, out: %s", err, out)
	}
	var init2 InitResponse
	if err := json.Unmarshal([]byte(out), &init2); err != nil {
		t.Fatalf("failed to parse init 2 JSON: %v", err)
	}
	initHMap, ok := init2.LastHandoff.(map[string]any)
	if !ok || initHMap["summary"] != "Phase 1 complete" {
		t.Errorf("expected structured handoff on turn 1 init, got: %+v", init2.LastHandoff)
	}
}
