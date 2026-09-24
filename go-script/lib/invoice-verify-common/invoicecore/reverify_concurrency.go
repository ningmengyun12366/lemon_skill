package invoicecore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/excelx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/verifyapi"
)

const reverifyConcurrency = 10

type reverifyJob struct {
	index   int
	request verifyapi.Request
}

type reverifyWorkerError struct {
	row int
	err error
}

type reverifyJobRunner struct {
	rows     []map[string]string
	verifier Verifier
	now      func() time.Time
}

func runReverifyJobs(ctx context.Context, jobs []reverifyJob, runner reverifyJobRunner) error {
	if len(jobs) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobCh := make(chan reverifyJob)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr *reverifyWorkerError

	for worker := 0; worker < reverifyConcurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				if err := runner.run(ctx, job); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = &reverifyWorkerError{row: job.index + 2, err: err}
						cancel()
					}
					mu.Unlock()
				}
			}
		}()
	}

	for _, job := range jobs {
		select {
		case jobCh <- job:
		case <-ctx.Done():
			close(jobCh)
			wg.Wait()
			return firstReverifyError(firstErr, ctx.Err())
		}
	}
	close(jobCh)
	wg.Wait()
	return firstReverifyError(firstErr, nil)
}

func (runner reverifyJobRunner) run(ctx context.Context, job reverifyJob) error {
	row := runner.rows[job.index]
	row[excelx.VerifyTimeHeader] = runner.now().Format("2006-01-02 15:04:05")
	result, err := runner.verifier.Verify(ctx, job.request)
	if err != nil {
		return err
	}
	if err := applyReverifyResult(row, result); err != nil {
		return fmt.Errorf("result: %w", err)
	}
	return nil
}

func firstReverifyError(firstErr *reverifyWorkerError, fallback error) error {
	if firstErr != nil {
		return fmt.Errorf("reverify row %d: %w", firstErr.row, firstErr.err)
	}
	if fallback != nil {
		return fallback
	}
	return nil
}
