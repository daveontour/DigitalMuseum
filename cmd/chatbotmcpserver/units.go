package main

import (
	"fmt"
	"strings"
)

// lengthToMeters and weightToGrams give each supported unit's factor to a common base unit, so
// any-to-any conversion is just (value * fromFactor) / toFactor.
var lengthToMeters = map[string]float64{
	"mm": 0.001,
	"cm": 0.01,
	"m":  1,
	"km": 1000,
	"in": 0.0254,
	"ft": 0.3048,
	"yd": 0.9144,
	"mi": 1609.344,
}

var weightToGrams = map[string]float64{
	"g":  1,
	"kg": 1000,
	"oz": 28.349523125,
	"lb": 453.59237,
}

var temperatureUnits = map[string]bool{"c": true, "f": true, "k": true}

// convertUnits converts value between from/to, auto-detecting which family (length, weight,
// temperature) both units belong to. Units are matched case-insensitively.
func convertUnits(value float64, from, to string) (float64, error) {
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))

	if fromF, ok := lengthToMeters[from]; ok {
		toF, ok := lengthToMeters[to]
		if !ok {
			return 0, fmt.Errorf("%q is not a recognized length unit for conversion from %q", to, from)
		}
		return value * fromF / toF, nil
	}
	if fromF, ok := weightToGrams[from]; ok {
		toF, ok := weightToGrams[to]
		if !ok {
			return 0, fmt.Errorf("%q is not a recognized weight unit for conversion from %q", to, from)
		}
		return value * fromF / toF, nil
	}
	if temperatureUnits[from] {
		if !temperatureUnits[to] {
			return 0, fmt.Errorf("%q is not a recognized temperature unit for conversion from %q", to, from)
		}
		return convertTemperature(value, from, to), nil
	}
	return 0, fmt.Errorf("%q is not a recognized unit (supported: length mm/cm/m/km/in/ft/yd/mi, weight g/kg/oz/lb, temperature c/f/k)", from)
}

func convertTemperature(value float64, from, to string) float64 {
	if from == to {
		return value
	}
	// Normalize to Celsius first, then convert to the target.
	var celsius float64
	switch from {
	case "c":
		celsius = value
	case "f":
		celsius = (value - 32) * 5 / 9
	case "k":
		celsius = value - 273.15
	}
	switch to {
	case "c":
		return celsius
	case "f":
		return celsius*9/5 + 32
	case "k":
		return celsius + 273.15
	}
	return celsius
}
