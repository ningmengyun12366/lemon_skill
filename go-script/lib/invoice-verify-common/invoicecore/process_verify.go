package invoicecore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoiceinput"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/verifyapi"
)

type initialVerifyStatus int

const (
	initialVerifySuccess initialVerifyStatus = iota
	initialVerifyFailed
	initialVerifyRateLimited
)

const initialVerifyConcurrency = 10

type initialVerifyJob struct {
	Index int
	Entry invoiceinput.Entry
}

type initialVerifyOutcome struct {
	Index  int
	Record IntermediateRecord
	Status initialVerifyStatus
}

type initialVerifyError struct {
	Index int
	Err   error
}

func runInitialVerifyJobs(ctx context.Context, jobs []initialVerifyJob, verifier Verifier, now func() time.Time) ([]initialVerifyOutcome, error) {
	if len(jobs) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	jobCh := make(chan initialVerifyJob)
	outcomeCh := make(chan initialVerifyOutcome, len(jobs))
	errorCh := make(chan initialVerifyError, len(jobs))
	workerCount := min(initialVerifyConcurrency, len(jobs))
	doneCh := make(chan struct{}, workerCount)
	for range workerCount {
		go func() {
			defer func() {
				doneCh <- struct{}{}
			}()
			for job := range jobCh {
				verifyTime := now().Format("2006-01-02 15:04:05")
				verifyResult, err := verifier.Verify(ctx, deriveVerifyRequest(job.Entry.QRData))
				if err != nil {
					if context.Cause(ctx) != nil && errors.Is(err, context.Canceled) {
						continue
					}
					wrapped := fmt.Errorf("第%d行查验失败: %w", job.Index+1, err)
					errorCh <- initialVerifyError{Index: job.Index, Err: wrapped}
					cancel(wrapped)
					continue
				}
				outcomeCh <- buildInitialVerifyOutcome(job, verifyResult, verifyTime)
			}
		}()
	}

sendLoop:
	for _, job := range jobs {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobCh <- job:
		}
	}
	close(jobCh)
	for range workerCount {
		<-doneCh
	}
	close(outcomeCh)
	close(errorCh)

	var firstErr error
	firstErrIndex := len(jobs)
	for verifyError := range errorCh {
		if verifyError.Index < firstErrIndex {
			firstErrIndex = verifyError.Index
			firstErr = verifyError.Err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}

	outcomes := make([]initialVerifyOutcome, 0, len(outcomeCh))
	for outcome := range outcomeCh {
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

func buildInitialVerifyOutcome(job initialVerifyJob, verifyResult verifyapi.Result, verifyTime string) initialVerifyOutcome {
	if verifyResult.Success {
		return initialVerifyOutcome{
			Index:  job.Index,
			Record: buildRecord(job.Entry, &verifyResult, "查验成功", verifyTime),
			Status: initialVerifySuccess,
		}
	}
	if verifyResult.IsRateLimited {
		return initialVerifyOutcome{
			Index:  job.Index,
			Record: buildRecord(job.Entry, nil, verifyFailureStatus(verifyResult), verifyTime),
			Status: initialVerifyRateLimited,
		}
	}
	return initialVerifyOutcome{
		Index:  job.Index,
		Record: buildRecord(job.Entry, nil, verifyFailureStatus(verifyResult), verifyTime),
		Status: initialVerifyFailed,
	}
}
