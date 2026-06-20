package downstreamb

import "encoding/json"

// jsonUnmarshalImpl 抽出来避免循环 import
func jsonUnmarshalImpl(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
