package nvidia

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"regexp"
	"strconv"
)

type swapMetricsSender struct {
	regex *regexp.Regexp
}

func (swapMetricsSender *swapMetricsSender) Init() error {
	regex, err := regexp.Compile(`SWAP\s*(?P<usedSwap>\d+)/(?P<totalSwap>\d+)(?P<swapUnit>[kKmMgG][bB])\s*\(cached\s*(?P<cached>\d+)(?P<cachedUnit>[kKmMgG][bB])\)`)
	if err != nil {
		return err
	}
	swapMetricsSender.regex = regex

	return nil
}

func (swapMetricsSender *swapMetricsSender) SendMetrics(sender aggregator.Sender, field string) error {
	swapFields := regexFindStringSubmatchMap(swapMetricsSender.regex, field)
	if swapFields == nil{
		// SWAP is not present on all devices
		return nil
	}

	swapMultiplier := getSizeMultiplier(swapFields["swapUnit"])

	swapUsed, err := strconv.ParseFloat(swapFields["usedSwap"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.swap.used", swapUsed*swapMultiplier, "", nil)

	totalSwap, err := strconv.ParseFloat(swapFields["totalSwap"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.swap.total", totalSwap*swapMultiplier, "", nil)

	cacheMultiplier := getSizeMultiplier(swapFields["cachedUnit"])
	cached, err := strconv.ParseFloat(swapFields["cached"], 64)
	if err != nil {
		return err
	}
	sender.Gauge("nvidia.jetson.gpu.swap.cached", cached*cacheMultiplier, "", nil)

	return nil
}
