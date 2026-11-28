package music

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Decode result into its destination without copying both envelope and result.
type apiResult struct {
	out     any
	present bool
}

func (r *apiResult) UnmarshalJSON(data []byte) error {
	r.present = !bytes.Equal(bytes.TrimSpace(data), []byte("null"))
	if r.present && r.out != nil {
		return json.Unmarshal(data, r.out)
	}
	return nil
}
func decodeAPIResponse(reader io.Reader, rotor bool, out any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, 16<<20))
	if rotor {
		// Rotor supports direct and wrapped responses; its batches are small.
		var raw json.RawMessage
		if e := decoder.Decode(&raw); e != nil {
			return errors.New("не удалось прочитать ответ Яндекса")
		}
		var envelope struct {
			Result json.RawMessage `json:"result"`
			Error  apiResult       `json:"error"`
		}
		if e := json.Unmarshal(raw, &envelope); e != nil {
			return errors.New("не удалось прочитать ответ Яндекса")
		}
		if envelope.Error.present {
			return errors.New("Яндекс отклонил запрос")
		}
		if out == nil {
			return nil
		}
		payload := envelope.Result
		if len(payload) == 0 {
			payload = raw
		}
		if bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
			return errors.New("пустой ответ Яндекса")
		}
		return json.Unmarshal(payload, out)
	}
	var envelope struct {
		Result apiResult `json:"result"`
		Error  apiResult `json:"error"`
	}
	envelope.Result.out = out
	if e := decoder.Decode(&envelope); e != nil {
		return errors.New("не удалось прочитать ответ Яндекса")
	}
	if envelope.Error.present {
		return errors.New("Яндекс отклонил запрос")
	}
	if out != nil && !envelope.Result.present {
		return errors.New("пустой ответ Яндекса")
	}
	return nil
}
