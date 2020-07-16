// Copyright 2019 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package transform

import (
	"errors"

	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"go.opentelemetry.io/otel/api/label"
	"go.opentelemetry.io/otel/api/metric"
	metricsdk "go.opentelemetry.io/otel/sdk/export/metric"
	"go.opentelemetry.io/otel/sdk/export/metric/aggregation"
	"go.opentelemetry.io/otel/sdk/resource"
)

// ErrUnimplementedAgg is returned when a transformation of an unimplemented
// aggregator is attempted.
var ErrUnimplementedAgg = errors.New("unimplemented aggregator")

// Record transforms an OpenTelemetry Record into a Metric.
//
// An ErrUnimplementedAgg error is returned for unimplemented Aggregators.
func Record(service string, res *resource.Resource, record metricsdk.Record) (telemetry.Metric, error) {
	desc := record.Descriptor()
	attrs := attributes(service, res, desc, record.Labels())
	switch a := record.Aggregation().(type) {
	case aggregation.MinMaxSumCount:
		return minMaxSumCount(desc, attrs, a)
	case aggregation.Sum:
		return sum(desc, attrs, a)
	}
	return nil, ErrUnimplementedAgg
}

// TODO see also https://github.com/open-telemetry/opentelemetry-go/blob/4f3fab3ba7df677205e673ae743ee067c99dbe87/exporters/otlp/ for concurrent implementation
// sum transforms a Sum Aggregator aggregation into a Count Metric.
func sum(desc *metric.Descriptor, attrs map[string]interface{}, a aggregation.Sum) (telemetry.Metric, error) {
	sum, err := a.Sum()
	if err != nil {
		return nil, err
	}

	return telemetry.Count{
		Name:       desc.Name(),
		Attributes: attrs,
		Value:      sum.CoerceToFloat64(desc.NumberKind()),
	}, nil
}

// minMaxSumCountValue returns the values of the MinMaxSumCount Aggregator
// as discret values or any error returned from parsing any of the values.
func minMaxSumCountValues(a aggregation.MinMaxSumCount) (min, max, sum metric.Number, count int64, err error) {
	if min, err = a.Min(); err != nil {
		return
	}
	if max, err = a.Max(); err != nil {
		return
	}
	if sum, err = a.Sum(); err != nil {
		return
	}
	if count, err = a.Count(); err != nil {
		return
	}
	return
}

// minMaxSumCount transforms a MinMaxSumCount aggregation into a Summary Metric.
func minMaxSumCount(desc *metric.Descriptor, attrs map[string]interface{}, a aggregation.MinMaxSumCount) (telemetry.Metric, error) {
	min, max, sum, count, err := minMaxSumCountValues(a)
	if err != nil {
		return nil, err
	}

	return telemetry.Summary{
		Name:       desc.Name(),
		Attributes: attrs,
		Count:      float64(count),
		Sum:        sum.CoerceToFloat64(desc.NumberKind()),
		Min:        min.CoerceToFloat64(desc.NumberKind()),
		Max:        max.CoerceToFloat64(desc.NumberKind()),
	}, nil
}

func attributes(service string, res *resource.Resource, desc *metric.Descriptor, labels *label.Set) map[string]interface{} {
	// By default include New Relic attributes and all labels
	n := 2 + labels.Len() + res.Len()
	if desc != nil {
		if desc.Unit() != "" {
			n++
		}
		if desc.Description() != "" {
			n++
		}
	}
	if service != "" {
		n++
	}
	attrs := make(map[string]interface{}, n)

	for iter := res.Iter(); iter.Next(); {
		kv := iter.Label()
		attrs[string(kv.Key)] = kv.Value.AsInterface()
	}

	// If duplicate labels with Resource these take precedence.
	for iter := labels.Iter(); iter.Next(); {
		kv := iter.Label()
		attrs[string(kv.Key)] = kv.Value.AsInterface()
	}

	if desc != nil {
		if desc.Unit() != "" {
			attrs["unit"] = string(desc.Unit())
		}
		if desc.Description() != "" {
			attrs["description"] = desc.Description()
		}
	}
	if service != "" {
		attrs[serviceNameAttrKey] = service
	}

	// New Relic registered attributes to identify where this data came from.
	attrs[instrumentationProviderAttrKey] = instrumentationProviderAttrValue
	attrs[collectorNameAttrKey] = collectorNameAttrValue

	return attrs
}
