package wagozlib

import "strconv"

// Status is the stable result code returned by the Go API and the guest ABI.
// StatusOK is the only status for which output bytes and the written count are
// committed to the destination.
type Status uint32

const (
	StatusOK Status = iota
	StatusInvalidArgument
	StatusInputTooLarge
	StatusOutputTooLarge
	StatusOutputTooSmall
	StatusInvalidData
	StatusTruncated
	StatusChecksumMismatch
	StatusDictionaryRequired
	StatusTrailingData
	StatusBusy
	StatusUnsupported
	StatusInternalError
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusInvalidArgument:
		return "invalid argument"
	case StatusInputTooLarge:
		return "input too large"
	case StatusOutputTooLarge:
		return "output too large"
	case StatusOutputTooSmall:
		return "output too small"
	case StatusInvalidData:
		return "invalid zlib data"
	case StatusTruncated:
		return "truncated zlib data"
	case StatusChecksumMismatch:
		return "zlib checksum mismatch"
	case StatusDictionaryRequired:
		return "preset dictionary required"
	case StatusTrailingData:
		return "trailing data"
	case StatusBusy:
		return "busy"
	case StatusUnsupported:
		return "unsupported guest ABI"
	case StatusInternalError:
		return "internal error"
	default:
		return "status(" + strconv.FormatUint(uint64(s), 10) + ")"
	}
}
