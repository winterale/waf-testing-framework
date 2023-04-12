package logs

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

//Formatter defines the format of log output
type Formatter struct {
	TimestampFormat string
}

// Format an log entry
func (f *Formatter) Format(entry *logrus.Entry) ([]byte, error) {

	timestampFormat := f.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = time.StampMilli
	}

	// output buffer
	b := &bytes.Buffer{}

	// write time
	b.WriteString(entry.Time.Format(timestampFormat))

	// write level
	level := strings.ToUpper(entry.Level.String())

	b.WriteString(" [" + level + "] ")

	// write message
	b.WriteString(strings.TrimSpace(entry.Message) + " ")

	// write calling function
	if entry.HasCaller() {
		fmt.Fprintf(
			b,
			" (%s:%d %s)",
			entry.Caller.File,
			entry.Caller.Line,
			entry.Caller.Function,
		)
	}

	// write fields
	f.writeFields(b, entry)

	b.WriteByte('\n')

	return b.Bytes(), nil
}

func (f *Formatter) writeFields(b *bytes.Buffer, entry *logrus.Entry) {
	if len(entry.Data) != 0 {
		fields := make([]string, 0, len(entry.Data))
		for field := range entry.Data {
			fields = append(fields, field)
		}

		sort.Strings(fields)

		for _, field := range fields {
			fmt.Fprintf(b, "[%s:%v] ", field, entry.Data[field])

		}
	}
}
