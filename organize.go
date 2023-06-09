package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"text/template"
	"time"

	"golang.org/x/exp/slog"
)

// An Organizer organizes plots into a dated directory hierarchy
// Plots will be placed into a folder named as base/{year}/{month}/{day}
// Hourly plots will be placed in a subfolder named {hour}
// If the plot is determined to be the latest version then it will be
// copied to a directory called "latest"
// So a plot called demo.json dated 2023-05-08 will be placed in:
//
//	base/2023/05/08/demo.json
//	latest/demo.json
type Organizer struct {
	Base     string
	Template string
	Params   map[string]any
}

func (o *Organizer) Filename(name string) (string, error) {
	t, err := template.New("").Parse(o.Template)
	if err != nil {
		return "", fmt.Errorf("parsing filename template: %w", err)
	}

	data := map[string]any{
		"Params":          o.Params,
		"PlotDefFilename": name,
	}

	buf := new(bytes.Buffer)
	if err := t.Execute(buf, data); err != nil {
		return "", fmt.Errorf("execute filename template: %w", err)
	}

	return buf.String(), nil
}

func (o *Organizer) Filepath(pd *PlotDef, basisTime time.Time) (string, error) {
	var dated string
	switch pd.Frequency {
	case PlotFrequencyWeekly:
		dated = pd.Frequency.Truncate(basisTime).Format("2006/01/02")
	case PlotFrequencyDaily:
		dated = pd.Frequency.Truncate(basisTime).Format("2006/01/02")
	case PlotFrequencyHourly:
		dated = pd.Frequency.Truncate(basisTime).Format("2006/01/02/15")
	default:
		slog.Warn(fmt.Sprintf("unsupported plot frequency: %q", pd.Frequency))
	}

	filename, err := o.Filename(pd.Name)
	if err != nil {
		return "", err
	}

	return filepath.Join(o.Base, dated, filename), nil
}

func (o *Organizer) Glob(pd *PlotDef, basisTime time.Time) ([]string, error) {
	var pattern string
	switch pd.Frequency {
	case PlotFrequencyWeekly:
		pattern = "20[0-9][0-9]/[0-9][0-9]/[0-9][0-9]"
	case PlotFrequencyDaily:
		pattern = "20[0-9][0-9]/[0-9][0-9]/[0-9][0-9]"
	case PlotFrequencyHourly:
		pattern = "20[0-9][0-9]/[0-9][0-9]/[0-9][0-9]/[0-9][0-9]"
	default:
		slog.Warn(fmt.Sprintf("unsupported plot frequency: %q", pd.Frequency))
	}
	pattern = filepath.Join(o.Base, pattern, pd.Name+".json")

	return filepath.Glob(pattern)
}

func (o *Organizer) LatestFilepath(pd *PlotDef) (string, error) {
	filename, err := o.Filename(pd.Name)
	if err != nil {
		return "", err
	}

	return filepath.Join(o.Base, "latest", filename), nil
}

func (o *Organizer) IsStaleOrMissing(pd *PlotDef, basisTime time.Time, expectedTime time.Time) (bool, error) {
	fname, err := o.Filepath(pd, basisTime)
	if err != nil {
		return false, fmt.Errorf("filepath: %w", err)
	}

	info, err := os.Lstat(fname)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("stat file: %w", err)
	}

	return info.ModTime().Before(expectedTime), nil
}

func (o *Organizer) IsLatest(pd *PlotDef, basisTime time.Time) (bool, error) {
	existing, err := o.Glob(pd, basisTime)
	if err != nil {
		return false, fmt.Errorf("glob: %w", err)
	}

	// add the current filename to the existing ones, sort and see if current
	// filename is the last entry
	fname, _ := o.Filepath(pd, basisTime)
	existing = append(existing, fname)
	sort.Strings(existing)
	if existing[len(existing)-1] == fname {
		return true, nil
	}
	return false, nil
}

func (o *Organizer) WritePlot(data []byte, pd *PlotDef, basisTime time.Time) error {
	path, err := o.Filepath(pd, basisTime)
	if err != nil {
		return err
	}

	if err := writeOutput(path, data); err != nil {
		return fmt.Errorf("write plot: %w", err)
	}

	isLatest, err := o.IsLatest(pd, basisTime)
	if err != nil {
		return fmt.Errorf("is latest: %w", err)
	}
	if !isLatest {
		return nil
	}

	path, err = o.LatestFilepath(pd)
	if err != nil {
		return err
	}

	if err := writeOutput(path, data); err != nil {
		return fmt.Errorf("write latest: %w", err)
	}
	return nil
}
