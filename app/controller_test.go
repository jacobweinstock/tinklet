package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/packethost/pkg/log/logr"
	"github.com/pkg/errors"
)

func TestReportActionStatusController(t *testing.T) {
	t.Skip()
	ctx, cancel := context.WithCancel(context.Background())
	log, _, _ := logr.NewPacketLogr()
	var wg sync.WaitGroup
	var doneWg sync.WaitGroup
	rasChan := make(chan func() error)
	go ReportActionStatusController(ctx, log, &wg, rasChan, &doneWg)

	wg.Add(1)
	rasChan <- func() error {
		return errors.New("test error 1")
	}
	wg.Add(1)
	rasChan <- func() error {
		return errors.New("test error 2")
	}
	wg.Add(1)
	rasChan <- func() error {
		return errors.New("test error 3")
	}
	wg.Wait()
	cancel()
	time.Sleep(3 * time.Second)
	t.Fatal()
}
