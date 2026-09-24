package runtimeio

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/contract"
)

type Runtime struct {
	stdout io.Writer
	stderr io.Writer
}

func New(stdout, stderr io.Writer) *Runtime {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &Runtime{stdout: stdout, stderr: RedactingWriter(stderr)}
}

func (r *Runtime) Stdout() io.Writer {
	return r.stdout
}

func (r *Runtime) Stderr() io.Writer {
	return r.stderr
}

func (r *Runtime) WriteJSON(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := r.stdout.Write(payload); err != nil {
		return err
	}
	_, err = r.stdout.Write([]byte("\n"))
	return err
}

func (r *Runtime) WriteFailure(err error) error {
	message := "unexpected failure"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	return r.WriteJSON(contract.NewGenericFailure(Redact(message)))
}

func (r *Runtime) Logf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	message = Redact(message)
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	_, _ = r.stderr.Write([]byte(message))
}

type redactingWriter struct {
	writer io.Writer
}

func RedactingWriter(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return redactingWriter{writer: writer}
}

func (w redactingWriter) Write(payload []byte) (int, error) {
	_, err := w.writer.Write([]byte(Redact(string(payload))))
	return len(payload), err
}
