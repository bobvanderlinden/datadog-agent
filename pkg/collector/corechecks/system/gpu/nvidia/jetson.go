// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package nvidia

import (
	"bufio"
	"context"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	checkName = "jetson"

	// The interval to run tegrastats at, in ms
	tegraStatsInterval = 100

	kb = 1024
	mb = kb * 1024
	gb = mb * 1024
)

type checkCfg struct {
	TegraStatsPath string `yaml:"tegrastats_path,omitempty"`
}

// JetsonCheck contains the field for the JetsonCheck
type JetsonCheck struct {
	core.CheckBase
	metrics.Gauge
	// The path to the tegrastats binary. Defaults to /usr/bin/tegrastats
	tegraStatsPath string

	// The command line options for tegrastats
	commandOpts []string

	metricsSenders []metricsSender
}

func getSizeMultiplier(unit string) float64 {
	switch strings.ToLower(unit) {
	case "kb":
		return kb
	case "mb":
		return mb
	case "gb":
		return gb
	}
	return 1
}

func (c *JetsonCheck) processTegraStatsOutput(tegraStatsOuptut string) error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	for _, metricSender := range c.metricsSenders {
		err = metricSender.SendMetrics(sender, tegraStatsOuptut)
		if err != nil {
			return err
		}
	}
	sender.Commit()
	return nil
}

// Run executes the check
func (c *JetsonCheck) Run() error {
	// Kill tegrastats it it runs for twice as long as the interval we specified, to avoid blocking
	// the check forever
	ctx, cancel := context.WithTimeout(context.Background(), 2*tegraStatsInterval*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.tegraStatsPath, c.commandOpts...)

	// Parse the standard output for the stats
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stdout)
		if in.Scan() {
			// We only need to read one line
			line := in.Text()
			log.Debugf("tegrastats: %s", line)
			if err = c.processTegraStatsOutput(line); err != nil {
				_ = log.Error(err)
			}
		} else {
			_ = log.Warnf("tegrastats did not produce any output")
		}
		// Tegrastats keeps running forever, so kill it after trying to read
		// one line of output
		err = cmd.Process.Signal(os.Kill)
		if err != nil {
			_ = log.Errorf("unable to stop %s check: %s", checkName, err)
		}
	}()

	// forward the standard error to the Agent logger
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			_ = log.Error(in.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
		return err
	}

	// No need to check the result since we kill the process, so err is normally != nil
	_ = cmd.Wait()

	return nil
}

// Configure the GPU check
func (c *JetsonCheck) Configure(data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(data, source)
	if err != nil {
		return err
	}

	var conf checkCfg
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return err
	}
	if conf.TegraStatsPath != "" {
		c.tegraStatsPath = conf.TegraStatsPath
	} else {
		c.tegraStatsPath = "/usr/bin/tegrastats"
	}

	// We run tegrastats once and then kill the process. However, we set the interval to 500ms
	// because it will take tegrastats <interval> to produce its first output.
	c.commandOpts = []string{
		"--interval",
		strconv.Itoa(tegraStatsInterval), // ms
	}

	c.metricsSenders = []metricsSender {
		&cpuMetricSender{},
		&gpuMetricSender{},
		&iramMetricSender{},
		&ramMetricSender{},
		&swapMetricsSender{},
		&temperatureMetricsSender{},
		&voltageMetricsSender{},
	}

	for _, metricSender := range c.metricsSenders {
		err := metricSender.Init()
		if err != nil {
			return err
		}
	}

	return nil
}

func jetsonCheckFactory() check.Check {
	return &JetsonCheck{
		CheckBase: core.NewCheckBase(checkName),
	}
}

func init() {
	core.RegisterCheck(checkName, jetsonCheckFactory)
}
