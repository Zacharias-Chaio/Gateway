package engine

import (
	"errors"
	"testing"
	"time"
)

func TestCommMonitorSnapshotsByDeviceAndResetsSession(t *testing.T) {
	m := newCommMonitor(7, 3)
	started := time.Date(2026, time.July, 24, 10, 0, 0, 0, time.UTC)
	m.reset(started)
	m.tx(0, 1, "read", 1, []byte{0x01, 0x03})
	m.rx(0, 1, "read", 1, []byte{0x01, 0x03, 0x00}, 20*time.Millisecond)
	m.complete(0, 1, "read", 1, nil, 20*time.Millisecond)
	m.tx(1, 2, "write", 1, []byte{0x02, 0x06})
	m.complete(1, 2, "write", 1, errors.New("response timeout"), 3*time.Second)

	all := m.snapshot(-1, 0, 10)
	if all.Stats.Requests != 2 || all.Stats.Successful != 1 || all.Stats.Failed != 1 || all.Stats.ErrorRate != 50 {
		t.Fatalf("unexpected aggregate stats: %+v", all.Stats)
	}
	if len(all.Events) != 3 { // bounded to the latest three packet/error events
		t.Fatalf("unexpected event count: %d", len(all.Events))
	}

	device := m.snapshot(1, 0, 10)
	if device.Stats.Requests != 1 || device.Stats.Failed != 1 || len(device.Events) != 2 {
		t.Fatalf("unexpected device snapshot: %+v", device)
	}
	if device.Events[0].DeviceIndex != 1 || device.Events[1].Direction != "ERR" {
		t.Fatalf("device filter returned wrong events: %+v", device.Events)
	}
	if incremental := m.snapshot(-1, all.NextSeq, 10); len(incremental.Events) != 0 {
		t.Fatalf("afterSeq should exclude known events: %+v", incremental.Events)
	}

	m.reset(started.Add(time.Minute))
	reset := m.snapshot(-1, 0, 10)
	if !reset.Stats.StartedAt.Equal(started.Add(time.Minute)) || reset.Stats.Requests != 0 || len(reset.Events) != 0 {
		t.Fatalf("reset did not start a clean session: %+v", reset)
	}
}
