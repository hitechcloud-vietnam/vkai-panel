package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// GetEnv gets an environment variable with a default value
func GetEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// GetEnvInt gets an integer environment variable with a default value
func GetEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
 intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intValue
}

// GetEnvFloat gets a float environment variable with a default value
func GetEnvFloat(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	floatValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue
	}
	return floatValue
}

// GetEnvBool gets a boolean environment variable with a default value
func GetEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return boolValue
}

// GetEnvSlice gets a slice environment variable with a default value
func GetEnvSlice(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return strings.Split(value, ",")
}

// SetEnv sets an environment variable
func SetEnv(key, value string) error {
	return os.Setenv(key, value)
}

// UnsetEnv unsets an environment variable
func UnsetEnv(key string) error {
	return os.Unsetenv(key)
}

// HasEnv checks if an environment variable exists
func HasEnv(key string) bool {
	_, exists := os.LookupEnv(key)
	return exists
}

// LoadJSON loads a JSON file into a struct
func LoadJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	return json.Unmarshal(data, v)
}

// SaveJSON saves a struct to a JSON file
func SaveJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadJSONString loads JSON from a string into a struct
func LoadJSONString(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}

// SaveJSONString saves a struct to a JSON string
func SaveJSONString(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(data), nil
}

// MergeJSON merges two JSON objects
func MergeJSON(base, overlay map[string]interface{}) map[string]interface{} {
	for key, value := range overlay {
		if _, exists := base[key]; !exists {
			base[key] = value
		}
	}
	return base
}

// GetNestedValue gets a nested value from a map
func GetNestedValue(m map[string]interface{}, keys ...string) interface{} {
	current := m
	for i, key := range keys {
		if i == len(keys)-1 {
			return current[key]
		}
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else {
			return nil
		}
	}
	return nil
}

// SetNestedValue sets a nested value in a map
func SetNestedValue(m map[string]interface{}, value interface{}, keys ...string) {
	current := m
	for i, key := range keys {
		if i == len(keys)-1 {
			current[key] = value
			return
		}
		if _, exists := current[key]; !exists {
			current[key] = make(map[string]interface{})
		}
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		}
	}
}

// DeleteNestedValue deletes a nested value from a map
func DeleteNestedValue(m map[string]interface{}, keys ...string) {
	current := m
	for i, key := range keys {
		if i == len(keys)-1 {
			delete(current, key)
			return
		}
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else {
			return
		}
	}
}

// FlattenMap flattens a nested map
func FlattenMap(m map[string]interface{}, prefix string) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range m {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		if nested, ok := value.(map[string]interface{}); ok {
			for k, v := range FlattenMap(nested, fullKey) {
				result[k] = v
			}
		} else {
			result[fullKey] = value
		}
	}
	return result
}

// UnflattenMap unflattens a map
func UnflattenMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range m {
		keys := strings.Split(key, ".")
		current := result
		for i, k := range keys {
			if i == len(keys)-1 {
				current[k] = value
			} else {
				if _, exists := current[k]; !exists {
					current[k] = make(map[string]interface{})
				}
				current = current[k].(map[string]interface{})
			}
		}
	}
	return result
}
