// Package generationreport builds the report of files the generator wrote,
// with metrics (file count, total bytes, duration) and a human-readable
// summary.
//
// Every public function is wrapped by AppLogger.Logged.
//
// Since 1.4.0.
package generationreport

import (
	"fmt"
	"strings"
	"time"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/errors"
)

// File is one generated artefact. Since 1.4.0.
type File struct {
	Path string
	Bytes int
	BytesWritten int // bytes actually written to disk
}

// Report is the aggregate result of a generation run. Since 1.4.0.
type Report struct {
	ModulePath string
	OutputDir  string
	Files      []File
	StartedAt  time.Time
	FinishedAt time.Time
}

// New constructs an empty Report. Since 1.4.0.
func New(log *applogger.Logger) *Builder {
	if log == nil {
		log = applogger.New()
	}
	return &Builder{log: log, r: &Report{StartedAt: time.Now()}}
}

// Builder accumulates files into a Report. Since 1.4.0.
type Builder struct {
	log *applogger.Logger
	r   *Report
}

// Add records a generated file. Returns GEN-004 if the path is empty.
// Since 1.4.0.
func (b *Builder) Add(path string, content string) error {
	return b.log.Logged("GenerationReport.Add", func() error {
		if path == "" {
			b.log.Error("GenerationReport.Add: empty path")
			return errors.New(errors.CodeGeneration, "cannot add file with empty path")
		}
		written := len(content)
		f := File{Path: path, Bytes: written, BytesWritten: written}
		b.r.Files = append(b.r.Files, f)
		return nil
	})
}

// SetModule records the module path stamped into the generated go.mod.
// Since 1.4.0.
func (b *Builder) SetModule(mp string) {
	b.r.ModulePath = mp
}

// SetOutputDir records where the files were written. Since 1.4.0.
func (b *Builder) SetOutputDir(dir string) {
	b.r.OutputDir = dir
}

// Finalise stamps the finish time and returns the immutable Report.
// Since 1.4.0.
func (b *Builder) Finalise() (*Report, error) {
	return applogger.Logged1(b.log, "GenerationReport.Finalise", func() (*Report, error) {
		b.r.FinishedAt = time.Now()
		if len(b.r.Files) == 0 {
			b.log.Error("GenerationReport.Finalise: no files generated")
			return nil, errors.New(errors.CodeGeneration, "no files were generated")
		}
		// Defensive copy so callers cannot mutate the builder's slice.
		out := *b.r
		return &out, nil
	})
}

// FileCount is the number of files in the report. Since 1.4.0.
func (r *Report) FileCount() int { return len(r.Files) }

// TotalBytes is the sum of bytes across all files. Since 1.4.0.
func (r *Report) TotalBytes() int {
	var n int
	for _, f := range r.Files {
		n += f.Bytes
	}
	return n
}

// Duration is the wall-clock generation time. Since 1.4.0.
func (r *Report) Duration() time.Duration { return r.FinishedAt.Sub(r.StartedAt) }

// Summary renders a human-readable multi-line summary. Since 1.4.0.
func (r *Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Generated %d files (%d bytes) into %s in %s\n",
		r.FileCount(), r.TotalBytes(), r.OutputDir, r.Duration())
	for _, f := range r.Files {
		fmt.Fprintf(&b, "  %-40s %8d bytes\n", f.Path, f.Bytes)
	}
	return b.String()
}
