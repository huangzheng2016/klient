package klient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf16"
	"unicode/utf8"
)

func (f *TranscriptFrame) UnmarshalJSON(data []byte) error {
	type plain TranscriptFrame
	aux := struct {
		*plain
		Text      json.RawMessage `json:"text"`
		InputText json.RawMessage `json:"inputText"`
	}{plain: (*plain)(f)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	textUnits, err := decodeUTF16Field(aux.Text)
	if err != nil {
		return fmt.Errorf("text: %w", err)
	}
	if textUnits != nil {
		f.textUnits = textUnits
		f.Text = string(utf16.Decode(textUnits))
	}
	inputUnits, err := decodeUTF16Field(aux.InputText)
	if err != nil {
		return fmt.Errorf("inputText: %w", err)
	}
	if inputUnits != nil {
		f.inputTextUnits = inputUnits
		f.InputText = string(utf16.Decode(inputUnits))
	}
	return nil
}

func (op *transcriptOperation) UnmarshalJSON(data []byte) error {
	type plain transcriptOperation
	aux := struct {
		*plain
		Text json.RawMessage `json:"text"`
	}{plain: (*plain)(op)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	textUnits, err := decodeUTF16Field(aux.Text)
	if err != nil {
		return fmt.Errorf("text: %w", err)
	}
	if textUnits != nil {
		op.textUnits = textUnits
		op.Text = string(utf16.Decode(textUnits))
	}
	return nil
}

func decodeUTF16Field(data []byte) ([]uint16, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	if trimmed[0] != '"' {
		return nil, fmt.Errorf("expected JSON string")
	}
	units := make([]uint16, 0, len(trimmed))
	for i := 1; i < len(trimmed); {
		if trimmed[i] == '"' {
			if len(bytes.TrimSpace(trimmed[i+1:])) != 0 {
				return nil, fmt.Errorf("unexpected content after JSON string")
			}
			return units, nil
		}
		if trimmed[i] == '\\' {
			i++
			if i >= len(trimmed) {
				return nil, fmt.Errorf("unterminated escape")
			}
			switch trimmed[i] {
			case '"', '\\', '/':
				units = append(units, uint16(trimmed[i]))
				i++
			case 'b':
				units = append(units, '\b')
				i++
			case 'f':
				units = append(units, '\f')
				i++
			case 'n':
				units = append(units, '\n')
				i++
			case 'r':
				units = append(units, '\r')
				i++
			case 't':
				units = append(units, '\t')
				i++
			case 'u':
				if i+4 >= len(trimmed) {
					return nil, fmt.Errorf("short unicode escape")
				}
				value := uint16(0)
				for j := 1; j <= 4; j++ {
					digit, ok := hexDigit(trimmed[i+j])
					if !ok {
						return nil, fmt.Errorf("invalid unicode escape")
					}
					value = value<<4 | uint16(digit)
				}
				units = append(units, value)
				i += 5
			default:
				return nil, fmt.Errorf("invalid escape %q", trimmed[i])
			}
			continue
		}
		if trimmed[i] < 0x20 {
			return nil, fmt.Errorf("unescaped control character")
		}
		r, size := utf8.DecodeRune(trimmed[i:])
		if r == utf8.RuneError && size == 1 {
			units = append(units, uint16(utf8.RuneError))
			i++
			continue
		}
		units = append(units, utf16.Encode([]rune{r})...)
		i += size
	}
	return nil, fmt.Errorf("unterminated JSON string")
}

func hexDigit(b byte) (int, bool) {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0'), true
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10, true
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10, true
	default:
		return 0, false
	}
}

func transcriptFrameTextUnits(frame TranscriptFrame) []uint16 {
	if frame.textUnits != nil {
		return frame.textUnits
	}
	return utf16.Encode([]rune(frame.Text))
}

func transcriptFrameInputUnits(frame TranscriptFrame) []uint16 {
	if frame.inputTextUnits != nil {
		return frame.inputTextUnits
	}
	return utf16.Encode([]rune(frame.InputText))
}

func transcriptOperationTextUnits(op transcriptOperation) []uint16 {
	if op.textUnits != nil {
		return op.textUnits
	}
	return utf16.Encode([]rune(op.Text))
}

func transcriptUnitsDiff(previous, next []uint16) ([]uint16, []uint16) {
	if equalUTF16(previous, next) || utf16HasPrefix(previous, next) {
		return next, nil
	}
	if !utf16HasPrefix(next, previous) {
		return next, nil
	}
	return next, next[len(previous):]
}

func utf16HasPrefix(value, prefix []uint16) bool {
	if len(prefix) > len(value) {
		return false
	}
	return equalUTF16(value[:len(prefix)], prefix)
}

func takeUTF16Emission(pending, novel []uint16) (string, []uint16) {
	units := make([]uint16, 0, len(pending)+len(novel))
	units = append(units, pending...)
	units = append(units, novel...)
	var nextPending []uint16
	if len(units) > 0 && isHighSurrogate(units[len(units)-1]) {
		nextPending = []uint16{units[len(units)-1]}
		units = units[:len(units)-1]
	}
	return string(utf16.Decode(units)), nextPending
}

func trailingHighSurrogate(units []uint16) []uint16 {
	if len(units) == 0 || !isHighSurrogate(units[len(units)-1]) {
		return nil
	}
	return []uint16{units[len(units)-1]}
}

func isHighSurrogate(unit uint16) bool {
	return unit >= 0xd800 && unit <= 0xdbff
}

func cloneUTF16(units []uint16) []uint16 {
	return append([]uint16(nil), units...)
}
