package api

import (
	"strconv"
	"strings"
)

func parseScore(v string) (int64, error) {
	negative := strings.HasPrefix(v, "-")
	v = strings.TrimPrefix(v, "-")
	p := strings.Split(v, ".")
	if len(p) > 2 || len(p) == 0 {
		return 0, strconv.ErrSyntax
	}
	whole, e := strconv.ParseInt(p[0], 10, 32)
	if e != nil || whole < 0 || whole > 10 {
		return 0, strconv.ErrSyntax
	}
	fraction := int64(0)
	if len(p) == 2 {
		if len(p[1]) == 0 || len(p[1]) > 2 {
			return 0, strconv.ErrSyntax
		}
		f := p[1]
		if len(f) == 1 {
			f += "0"
		}
		fraction, e = strconv.ParseInt(f, 10, 16)
		if e != nil || fraction < 0 || fraction > 99 {
			return 0, strconv.ErrSyntax
		}
	}
	n := whole*100 + fraction
	if negative {
		n = -n
	}
	return n, nil
}
