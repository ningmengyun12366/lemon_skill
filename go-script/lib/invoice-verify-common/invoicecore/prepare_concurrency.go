package invoicecore

import "runtime"

var runtimeNumCPU = runtime.NumCPU

func prepareEntryWorkerCount(entryCount int) int {
	if entryCount == 0 {
		return 0
	}
	workerCount := runtimeNumCPU()
	if workerCount < 1 {
		workerCount = 1
	}
	return min(workerCount, entryCount)
}
