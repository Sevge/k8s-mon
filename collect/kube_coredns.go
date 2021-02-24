package collect

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/n9e/k8s-mon/config"
)

func DoKubeCoreDnsCollect(cg *config.Config, logger log.Logger, dataMap *HistoryMap, funcName string) {

	start := time.Now()
	metricUrlMap := GetServerSideAddr(cg.CoreDnsC, logger, dataMap, funcName)
	if len(metricUrlMap) == 0 {
		level.Error(logger).Log("msg", "GetServerSideAddrEmpty", "funcName:", funcName)
		return
	}

	coredns_dns_request_duration_seconds_bucket := "coredns_dns_request_duration_seconds_bucket"
	coredns_dns_request_duration_seconds_bucket_m := make(map[float64]float64)

	coredns_dns_response_size_bytes_bucket := "coredns_dns_response_size_bytes_bucket"
	coredns_dns_response_size_bytes_bucket_m := make(map[float64]float64)

	coredns_dns_request_duration_seconds_sum := "coredns_dns_request_duration_seconds_sum"
	coredns_dns_request_duration_seconds_count := "coredns_dns_request_duration_seconds_count"

	coredns_dns_response_size_bytes_sum := "coredns_dns_response_size_bytes_sum"
	coredns_dns_response_size_bytes_count := "coredns_dns_response_size_bytes_count"

	avg_m := make(map[string]map[string]float64)

	var metricList []dataobj.MetricValue
	index := 0
	allNum := len(metricUrlMap)
	for uniqueHost, murl := range metricUrlMap {
		tmp := *cg.CoreDnsC
		c := &tmp
		c.Addr = murl
		// 添加service_addr tag
		newtagsm := map[string]string{
			cg.MultiServerInstanceUniqueLabel: uniqueHost,
			cg.MultiFuncUniqueLabel:           funcName,
		}
		for k, v := range cg.AppendTags {
			newtagsm[k] = v
		}
		metrics, err := CurlTlsMetricsApi(logger, funcName, c, newtagsm, cg.Step, cg.TimeOutSeconds, false)

		if err != nil {
			level.Error(logger).Log("msg", "CurlTlsMetricsResError", "func_name", funcName, "err:", err, "seq", fmt.Sprintf("%d/%d", index, allNum), "addr", c.Addr)
			continue
		}
		if len(metrics) == 0 {
			level.Error(logger).Log("msg", "CurlTlsMetricsResEmpty", "func_name", funcName, "seq", fmt.Sprintf("%d/%d", index, allNum), "addr", c.Addr)
			continue
		}

		for _, metric := range metrics {

			switch metric.Metric {
			case coredns_dns_request_duration_seconds_bucket:

				upperBound := metric.TagsMap["le"]
				upperBoundV, _ := strconv.ParseFloat(upperBound, 64)
				coredns_dns_request_duration_seconds_bucket_m[upperBoundV] += metric.Value
				continue
			case coredns_dns_response_size_bytes_bucket:

				upperBound := metric.TagsMap["le"]
				upperBoundV, _ := strconv.ParseFloat(upperBound, 64)
				coredns_dns_response_size_bytes_bucket_m[upperBoundV] += metric.Value
				continue
			case coredns_dns_request_duration_seconds_sum:
				newName := strings.Split(metric.Metric, "_sum")[0]
				im, loaded := avg_m[newName]
				if !loaded {
					im = make(map[string]float64)
				}
				im["sum"] += metric.Value
				avg_m[newName] = im

			case coredns_dns_request_duration_seconds_count:
				newName := strings.Split(metric.Metric, "_count")[0]
				im, loaded := avg_m[newName]
				if !loaded {
					im = make(map[string]float64)
				}
				im["count"] += metric.Value
				avg_m[newName] = im
			case coredns_dns_response_size_bytes_sum:
				newName := strings.Split(metric.Metric, "_sum")[0]
				im, loaded := avg_m[newName]
				if !loaded {
					im = make(map[string]float64)
				}
				im["sum"] += metric.Value
				avg_m[newName] = im

			case coredns_dns_response_size_bytes_count:
				newName := strings.Split(metric.Metric, "_count")[0]
				im, loaded := avg_m[newName]
				if !loaded {
					im = make(map[string]float64)
				}
				im["count"] += metric.Value
				avg_m[newName] = im
			default:
				if strings.HasSuffix(metric.Metric, "_bucket") {
					continue
				}

			}
			//	common
			if metric.CounterType == config.METRIC_TYPE_COUNTER {
				metric.Metric = metric.Metric + config.COUNTER_TO_GAUGE_METRIC_NAME_SUFFIX
			}

			metric.Nid = cg.ServerSideNid
			metricList = append(metricList, metric)
		}
	}

	if len(metricList) == 0 {
		level.Error(logger).Log("msg", "CurlTlsMetricsResFinallyEmptyNotPush", "func_name", funcName)
		return
	}

	newtagsm := map[string]string{
		cg.MultiFuncUniqueLabel: funcName,
	}
	for k, v := range cg.AppendTags {
		newtagsm[k] = v
	}

	// 开始算quantile
	metricList = histogramDeltaWork(dataMap, coredns_dns_request_duration_seconds_bucket_m, newtagsm, funcName, coredns_dns_request_duration_seconds_bucket, cg.ServerSideNid, cg.Step, metricList)
	metricList = histogramDeltaWork(dataMap, coredns_dns_response_size_bytes_bucket_m, newtagsm, funcName, coredns_dns_response_size_bytes_bucket, cg.ServerSideNid, cg.Step, metricList)

	// 开始算平均值
	for mName, avgm := range avg_m {
		mm := avgCompute(avgm, cg.ServerSideNid, mName, cg.Step, newtagsm)
		metricList = append(metricList, mm...)

	}
	level.Debug(logger).Log("msg", "DoCollectSuccessfullyReadyToPush", "funcName", funcName, "metrics_num", len(metricList), "time_took_seconds", time.Since(start).Seconds())
	go PushWork(cg.PushServerAddr, cg.TimeOutSeconds, metricList, logger, funcName)

}
