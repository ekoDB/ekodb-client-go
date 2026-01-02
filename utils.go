package ekodb

import (
	"encoding/base64"
	"strconv"
	"time"
)

// FieldValue represents an ekoDB field with type and value
type FieldValue struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// GetValue extracts the value from an ekoDB field object.
// ekoDB returns fields as {"type": "String", "value": "..."} objects.
// This helper safely extracts the value or returns the input if it's not a field object.
//
// Example:
//
//	user, _ := client.FindByID("users", userID)
//	email := GetStringValue(user["email"])
//	age := GetIntValue(user["age"])
func GetValue(field interface{}) interface{} {
	if field == nil {
		return nil
	}

	// Try to cast to map[string]interface{} (JSON object)
	if fieldMap, ok := field.(map[string]interface{}); ok {
		if value, exists := fieldMap["value"]; exists {
			return value
		}
	}

	// If not a field object, return as-is
	return field
}

// GetStringValue extracts a string value from an ekoDB field
func GetStringValue(field interface{}) string {
	val := GetValue(field)
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

// GetIntValue extracts an int value from an ekoDB field.
// It returns the extracted int and a boolean indicating whether the conversion succeeded.
// Note: This function uses (int, bool) return signature for better error detection,
// unlike other getters which return zero values on failure. This allows callers to
// distinguish between actual zero values and conversion errors.
//
// Conversion behavior:
//   - int64: Converted to int. On 32-bit systems where int is 32 bits, values outside
//     the range [-2147483648, 2147483647] will overflow. The function now validates
//     this range and returns false for out-of-bounds values on 32-bit systems.
//   - float64: Truncates the decimal portion (e.g., 3.9 becomes 3, -2.7 becomes -2).
//     This is intentional for numeric type flexibility but callers should be aware
//     that precision is lost. Returns false if the value would overflow int range.
func GetIntValue(field interface{}) (int, bool) {
	val := GetValue(field)
	switch v := val.(type) {
	case int:
		return v, true
	case int64:
		// Check for overflow on 32-bit systems (where int is 32 bits)
		const maxInt = int(^uint(0) >> 1)
		const minInt = -maxInt - 1
		if v > int64(maxInt) || v < int64(minInt) {
			return 0, false
		}
		return int(v), true
	case float64:
		// Truncates decimal portion - document this behavior
		// Also check for overflow
		const maxInt = int(^uint(0) >> 1)
		const minInt = -maxInt - 1
		if v > float64(maxInt) || v < float64(minInt) {
			return 0, false
		}
		return int(v), true
	}
	return 0, false
}

// GetFloatValue extracts a float64 value from an ekoDB field
func GetFloatValue(field interface{}) float64 {
	val := GetValue(field)
	if f, ok := val.(float64); ok {
		return f
	}
	if i, ok := val.(int); ok {
		return float64(i)
	}
	if i, ok := val.(int64); ok {
		return float64(i)
	}
	return 0.0
}

// GetBoolValue extracts a bool value from an ekoDB field
func GetBoolValue(field interface{}) bool {
	val := GetValue(field)
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

// GetValues extracts values from multiple fields in a record.
// Useful for processing entire records returned from ekoDB.
//
// Example:
//
//	user, _ := client.FindByID("users", userID)
//	values := GetValues(user, []string{"email", "first_name", "status"})
//	email := GetStringValue(values["email"])
func GetValues(record map[string]interface{}, fields []string) map[string]interface{} {
	result := make(map[string]interface{})
	for _, field := range fields {
		if val, exists := record[field]; exists {
			result[field] = GetValue(val)
		}
	}
	return result
}

// GetDateTimeValue extracts a time.Time value from an ekoDB DateTime field.
// Supports time.Time values directly and RFC3339-formatted strings.
// Returns nil if the field is not a datetime or if string parsing fails.
func GetDateTimeValue(field interface{}) *time.Time {
	val := GetValue(field)
	if t, ok := val.(time.Time); ok {
		return &t
	}
	if str, ok := val.(string); ok {
		if t, err := time.Parse(time.RFC3339, str); err == nil {
			return &t
		}
	}
	return nil
}

// GetUUIDValue extracts a string UUID from an ekoDB UUID field
func GetUUIDValue(field interface{}) string {
	val := GetValue(field)
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

// GetDecimalValue extracts a float64 from an ekoDB Decimal field.
// Accepts underlying values of type float64, int, int64, or a string
// containing a decimal representation. If conversion fails, it returns 0.0.
// This function extends GetFloatValue by adding support for string parsing.
func GetDecimalValue(field interface{}) float64 {
	// First try the standard float conversion
	if result := GetFloatValue(field); result != 0.0 {
		return result
	}

	// Handle string case (not covered by GetFloatValue)
	val := GetValue(field)
	if str, ok := val.(string); ok {
		if f, err := strconv.ParseFloat(str, 64); err == nil {
			return f
		}
	}

	return 0.0
}

// GetDurationValue extracts a time.Duration from an ekoDB Duration field.
// It accepts the following underlying value formats:
//   - time.Duration: returned as-is.
//   - int64: interpreted as a time.Duration value in nanoseconds.
//   - float64: interpreted as a duration in nanoseconds after truncation to int64.
//     Warning: Fractional nanoseconds are truncated (e.g., 1.5 becomes 1 nanosecond).
//     Large float64 values may overflow when converted to int64.
//   - map[string]interface{} with "secs" and "nanos" float64 fields:
//     {"secs": <seconds>, "nanos": <nanoseconds>}
//     Both secs and nanos are truncated from float64 to int64, losing any fractional
//     components (e.g., 1.5 seconds becomes 1 second). This is converted to
//     secs*time.Second + nanos*time.Nanosecond.
//
// Note: This function returns 0 for both an actual zero duration and invalid/malformed
// duration objects. Callers cannot distinguish between these cases. If the source of
// duration data is untrusted or may be corrupted, consider validating the structure
// before calling this function, or checking the raw field value first.
//
// For any unsupported or invalid format, it returns 0.
func GetDurationValue(field interface{}) time.Duration {
	val := GetValue(field)
	if d, ok := val.(time.Duration); ok {
		return d
	}
	if i, ok := val.(int64); ok {
		return time.Duration(i)
	}
	if f, ok := val.(float64); ok {
		// Truncates fractional nanoseconds
		return time.Duration(int64(f))
	}
	// Check for object with secs and nanos
	if m, ok := val.(map[string]interface{}); ok {
		secsVal, hasSecs := m["secs"]
		nanosVal, hasNanos := m["nanos"]
		if !hasSecs || !hasNanos {
			// Missing expected fields; treat as invalid duration
			// Note: Returns 0, indistinguishable from valid zero duration
			return 0
		}
		secs, okSecs := secsVal.(float64)
		nanos, okNanos := nanosVal.(float64)
		if !okSecs || !okNanos {
			// Incorrect field types; treat as invalid duration
			return 0
		}
		// Truncates fractional seconds and nanoseconds
		return time.Duration(int64(secs))*time.Second + time.Duration(int64(nanos))*time.Nanosecond
	}
	return 0
}

// GetBytesValue extracts a []byte from an ekoDB Bytes field.
// The underlying field value may be:
//   - []byte: returned as-is
//   - []interface{}: each element must be a numeric value (typically float64)
//     in the range [0, 255], which is converted to a byte. If any element is
//     not numeric or is out of range, the function returns nil.
//   - string: interpreted as a base64-encoded representation of the bytes
//     and decoded using base64.StdEncoding. If decoding fails, the function
//     returns nil.
//
// For any other type or conversion error, GetBytesValue returns nil.
// Note: Returning nil can mean either a nil field or a conversion error.
func GetBytesValue(field interface{}) []byte {
	val := GetValue(field)
	if b, ok := val.([]byte); ok {
		return b
	}
	if arr, ok := val.([]interface{}); ok {
		result := make([]byte, len(arr))
		for i, v := range arr {
			num, ok := v.(float64)
			if !ok {
				// Invalid element type; fail the conversion to avoid silent zero bytes
				return nil
			}
			if num < 0 || num > 255 {
				// Out-of-range byte value; fail the conversion
				return nil
			}
			result[i] = byte(num)
		}
		return result
	}
	if str, ok := val.(string); ok {
		// Assume base64
		if decoded, err := base64.StdEncoding.DecodeString(str); err == nil {
			return decoded
		}
	}
	return nil
}

// GetBinaryValue extracts a []byte from an ekoDB Binary field.
//
// This function is a thin wrapper around GetBytesValue and is currently
// functionally identical to it. ekoDB may expose both "Binary" and "Bytes"
// field types in schemas, but in this client implementation they are
// represented and decoded in the same way (as []byte).
//
// Having both GetBinaryValue and GetBytesValue allows callers to choose the
// helper that best matches their logical schema (e.g., using GetBinaryValue
// for fields declared as "Binary" in ekoDB), while keeping a single shared
// implementation for the actual conversion logic.
func GetBinaryValue(field interface{}) []byte {
	return GetBytesValue(field)
}

// GetArrayValue extracts a []interface{} from an ekoDB Array field
func GetArrayValue(field interface{}) []interface{} {
	val := GetValue(field)
	if arr, ok := val.([]interface{}); ok {
		return arr
	}
	return nil
}

// GetSetValue extracts a []interface{} from an ekoDB Set field.
//
// This function is intentionally identical to GetArrayValue and does not validate or enforce
// element uniqueness. ekoDB enforces set semantics (uniqueness) on the server side, so data
// retrieved directly from ekoDB is expected to already be unique. If the source of the data is
// untrusted, manually constructed, or may be corrupted, callers should not rely on this
// guarantee and should perform their own uniqueness validation or deduplication if required.
func GetSetValue(field interface{}) []interface{} {
	return GetArrayValue(field)
}

// GetVectorValue extracts a []float64 from an ekoDB Vector field.
// Returns nil if any element cannot be converted to float64 to ensure vector dimension integrity.
// Note: Returning nil can mean either a nil field or a conversion error, which is important
// for vector operations where dimension integrity is critical.
func GetVectorValue(field interface{}) []float64 {
	val := GetValue(field)
	if arr, ok := val.([]interface{}); ok {
		result := make([]float64, len(arr))
		for i, v := range arr {
			if f, ok := v.(float64); ok {
				result[i] = f
			} else if num, ok := v.(int); ok {
				result[i] = float64(num)
			} else if num, ok := v.(int64); ok {
				result[i] = float64(num)
			} else {
				// Invalid element type; fail conversion to preserve vector dimensions
				return nil
			}
		}
		return result
	}
	return nil
}

// GetObjectValue extracts a map[string]interface{} from an ekoDB Object field
func GetObjectValue(field interface{}) map[string]interface{} {
	val := GetValue(field)
	if m, ok := val.(map[string]interface{}); ok {
		return m
	}
	return nil
}

// ExtractRecord transforms an entire record by extracting all field values.
// Preserves the 'id' field and extracts values from all other fields.
//
// Example:
//
//	user, _ := client.FindByID("users", userID)
//	plainUser := ExtractRecord(user)
//	// plainUser is now map[string]interface{} with plain values
func ExtractRecord(record map[string]interface{}) map[string]interface{} {
	if record == nil {
		return nil
	}

	result := make(map[string]interface{})
	for key, value := range record {
		if key == "id" {
			result[key] = value
		} else {
			result[key] = GetValue(value)
		}
	}
	return result
}
