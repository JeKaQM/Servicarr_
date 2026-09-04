package resources

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const maxCPUIndex = 4095

var cpuCoreSensorPattern = regexp.MustCompile(`(?i)^(?:cpu\s+)?core\s*#?\s*(\d+)$`)

// populateCPUTelemetry keeps logical-CPU load and hardware temperature sensors
// separate unless Glances provides an explicit per-core sensor label.
func populateCPUTelemetry(snapshot *Snapshot, sensors []glancesSensor, perCPU []glancesPerCPU) {
	if snapshot == nil {
		return
	}

	loads := make(map[uint64]float64)
	maxLoadIndex := -1
	for _, reading := range perCPU {
		index := asUint64Ptr(reading.CPUNumber)
		load := asFloatPtr(reading.Total)
		if index == nil || load == nil || *index > maxCPUIndex || !isFinite(*load) {
			continue
		}
		loads[*index] = *load
		if int(*index) > maxLoadIndex {
			maxLoadIndex = int(*index)
		}
	}

	if maxLoadIndex >= 0 {
		legacyLoads := make([]float64, maxLoadIndex+1)
		for index, load := range loads {
			legacyLoads[int(index)] = load
		}
		snapshot.CPUPerCorePercent = legacyLoads
	}

	coreTemperatures := make(map[uint64]float64)
	averagePriority := 6
	averageSum := 0.0
	averageCount := 0
	for _, sensor := range sensors {
		value := cpuTemperatureValue(sensor)
		if value == nil {
			continue
		}

		label, _ := sensor.Label.(string)
		label = strings.TrimSpace(label)
		priority, isCPU := cpuTemperaturePriority(label)
		if !isCPU {
			continue
		}

		if index, ok := cpuCoreIndexFromSensorLabel(label); ok {
			coreTemperatures[index] = *value
		} else {
			snapshot.CPUTemperatureSensors = append(snapshot.CPUTemperatureSensors, CPUTemperatureSensor{
				Label: label,
				TempC: *value,
			})
		}

		if priority < averagePriority {
			averagePriority = priority
			averageSum = *value
			averageCount = 1
		} else if priority == averagePriority {
			averageSum += *value
			averageCount++
		}
	}

	if averageCount > 0 {
		average := averageSum / float64(averageCount)
		snapshot.CPUAvgTempC = &average
	}
	sort.Slice(snapshot.CPUTemperatureSensors, func(i, j int) bool {
		return strings.ToLower(snapshot.CPUTemperatureSensors[i].Label) < strings.ToLower(snapshot.CPUTemperatureSensors[j].Label)
	})

	indices := make(map[uint64]struct{}, len(loads)+len(coreTemperatures))
	for index := range loads {
		indices[index] = struct{}{}
	}
	for index := range coreTemperatures {
		indices[index] = struct{}{}
	}

	orderedIndices := make([]uint64, 0, len(indices))
	for index := range indices {
		orderedIndices = append(orderedIndices, index)
	}
	sort.Slice(orderedIndices, func(i, j int) bool { return orderedIndices[i] < orderedIndices[j] })

	for _, index := range orderedIndices {
		metric := CPUCoreMetric{Index: index}
		if value, ok := loads[index]; ok {
			load := value
			metric.LoadPercent = &load
		}
		if value, ok := coreTemperatures[index]; ok {
			temperature := value
			metric.TempC = &temperature
		}
		snapshot.CPUCoreMetrics = append(snapshot.CPUCoreMetrics, metric)
	}
}

func cpuTemperatureValue(sensor glancesSensor) *float64 {
	if sensorType, ok := sensor.Type.(string); ok {
		switch strings.ToLower(strings.TrimSpace(sensorType)) {
		case "", "temperature", "temperature_core":
		default:
			return nil
		}
	}
	if unit, ok := sensor.Unit.(string); ok {
		switch strings.ToLower(strings.TrimSpace(unit)) {
		case "", "c", "°c":
		default:
			return nil
		}
	}

	value := asFloatPtr(sensor.Value)
	if value == nil || !isFinite(*value) || *value < -100 || *value > 250 {
		return nil
	}
	return value
}

func cpuTemperaturePriority(label string) (int, bool) {
	normalized := strings.ToLower(strings.TrimSpace(label))
	if normalized == "" {
		return 0, false
	}
	if _, ok := cpuCoreIndexFromSensorLabel(normalized); ok {
		return 0, true
	}
	// Prefer the most specific temperature family available for the average.
	if strings.HasPrefix(normalized, "tccd") {
		return 1, true
	}
	if strings.HasPrefix(normalized, "tdie") {
		return 2, true
	}
	if strings.Contains(normalized, "package") {
		return 3, true
	}
	if strings.HasPrefix(normalized, "tctl") {
		return 4, true
	}
	if strings.Contains(normalized, "cpu") && !strings.Contains(normalized, "gpu") {
		return 5, true
	}
	return 0, false
}

func cpuCoreIndexFromSensorLabel(label string) (uint64, bool) {
	match := cpuCoreSensorPattern.FindStringSubmatch(strings.TrimSpace(label))
	if len(match) != 2 {
		return 0, false
	}
	index, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil || index > maxCPUIndex {
		return 0, false
	}
	return index, true
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
