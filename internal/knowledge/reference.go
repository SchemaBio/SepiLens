package knowledge

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

type sequence struct{ Offset, Length, LineBases, LineBytes int64 }
type Reference struct {
	File      string
	Sequences map[string]sequence
}

func openReference(file string) (*Reference, error) {
	f, e := os.Open(file)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	r := &Reference{File: file, Sequences: map[string]sequence{}}
	reader := bufio.NewReader(f)
	var offset int64
	var name string
	var current sequence
	var previous int64
	flush := func() {
		if name != "" {
			r.Sequences[name] = current
			r.Sequences[chromosome(name)] = current
		}
	}
	for {
		line, e := reader.ReadString('\n')
		if e != nil && e != io.EOF {
			return nil, e
		}
		if line == "" {
			break
		}
		clean := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(clean, ">") {
			flush()
			fields := strings.Fields(clean[1:])
			if len(fields) == 0 {
				return nil, errors.New("invalid FASTA header")
			}
			name = fields[0]
			current = sequence{Offset: offset + int64(len(line))}
			previous = 0
		} else {
			if name == "" || clean == "" {
				return nil, errors.New("invalid FASTA sequence")
			}
			if current.LineBases == 0 {
				current.LineBases = int64(len(clean))
				current.LineBytes = int64(len(line))
			} else if previous != current.LineBases {
				return nil, errors.New("variable FASTA line width")
			}
			for _, b := range clean {
				if !strings.ContainsRune("ACGTNacgtn", b) {
					return nil, errors.New("unsupported FASTA base")
				}
			}
			previous = int64(len(clean))
			current.Length += previous
		}
		offset += int64(len(line))
		if e == io.EOF {
			break
		}
	}
	flush()
	if len(r.Sequences) == 0 {
		return nil, errors.New("empty reference")
	}
	return r, nil
}
func chromosome(name string) string {
	base := strings.Split(name, ".")[0]
	if strings.HasPrefix(base, "NC_") {
		n, e := strconv.Atoi(strings.TrimPrefix(base, "NC_"))
		if e == nil {
			if n >= 1 && n <= 22 {
				return strconv.Itoa(n)
			}
			if n == 23 {
				return "X"
			}
			if n == 24 {
				return "Y"
			}
			if n == 12920 {
				return "MT"
			}
		}
	}
	return strings.TrimPrefix(name, "chr")
}
func (r *Reference) Bases(chrom string, start int64, length int) (string, error) {
	s, ok := r.Sequences[strings.TrimPrefix(chrom, "chr")]
	if !ok || start < 1 || length < 1 || length > 10000 || start-1+int64(length) > s.Length {
		return "", errors.New("reference coordinate unavailable")
	}
	f, e := os.Open(r.File)
	if e != nil {
		return "", e
	}
	defer f.Close()
	out := make([]byte, 0, length)
	pos := start - 1
	for len(out) < length {
		column := pos % s.LineBases
		n := s.LineBases - column
		if n > int64(length-len(out)) {
			n = int64(length - len(out))
		}
		chunk := make([]byte, n)
		_, e = f.ReadAt(chunk, s.Offset+(pos/s.LineBases)*s.LineBytes+column)
		if e != nil {
			return "", e
		}
		out = append(out, chunk...)
		pos += n
	}
	return strings.ToUpper(string(out)), nil
}
