package backup

import "io"

func writeStr(w io.Writer, str string) error {
	_, err := w.Write([]byte(str + "\x00"))
	return err
}

func readStr(r io.RuneReader) (string, error) {
	var str string
	for {
		chr, _, err := r.ReadRune()
		if err != nil {
			return "", err
		}
		if chr == '\x00' {
			return str, nil
		}
		str += string(chr)
	}
}
