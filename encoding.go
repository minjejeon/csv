package csv

import (
	"fmt"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
)

// WithEncoding sets a custom character encoding for decoding CSV data.
// It accepts any encoding.Encoding implementation from golang.org/x/text/encoding.
func WithEncoding(enc encoding.Encoding) Option {
	return func(r *Reader) {
		r.encoding = enc
	}
}

// WithCharset sets a character encoding by name (case-insensitive).
// Supported names include:
//   - Korean: "euc-kr", "cp949", "uhc"
//   - Japanese: "shift_jis", "sjis", "cp932", "euc-jp"
//   - Chinese: "gbk", "gb18030", "gb2312", "big5"
//   - Western: "iso-8859-1", "latin1", "windows-1252", "cp1252"
//   - Unicode: "utf-8", "utf-16le", "utf-16be"
func WithCharset(name string) Option {
	enc, err := lookupCharset(name)
	return func(r *Reader) {
		if err != nil {
			r.initErr = err
			return
		}
		r.encoding = enc
	}
}

func lookupCharset(name string) (encoding.Encoding, error) {
	canonical := strings.ToLower(strings.TrimSpace(name))
	canonical = strings.ReplaceAll(canonical, "-", "")
	canonical = strings.ReplaceAll(canonical, "_", "")

	switch canonical {
	case "euckr", "cp949", "uhc":
		return korean.EUCKR, nil
	case "shiftjis", "sjis", "cp932":
		return japanese.ShiftJIS, nil
	case "eucjp":
		return japanese.EUCJP, nil
	case "gbk":
		return simplifiedchinese.GBK, nil
	case "gb18030":
		return simplifiedchinese.GB18030, nil
	case "gb2312":
		return simplifiedchinese.HZGB2312, nil
	case "big5":
		return traditionalchinese.Big5, nil
	case "iso88591", "latin1":
		return charmap.ISO8859_1, nil
	case "windows1252", "cp1252":
		return charmap.Windows1252, nil
	case "utf8":
		return unicode.UTF8, nil
	case "utf16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), nil
	case "utf16be":
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), nil
	default:
		return nil, fmt.Errorf("csv: unsupported charset %q", name)
	}
}
