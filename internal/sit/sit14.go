// StuffIt file archiver client

// XAD library system for archive handling
// Copyright (C) 1998 and later by Dirk Stoecker <soft@dstoecker.de>

// little based on macutils 2.0b3 macunpack by Dik T. Winter
// Copyright (C) 1992 Dik T. Winter <dik@cwi.nl>

// algorithm 15 is based on the work of  Matthew T. Russotto
// Copyright (C) 2002 Matthew T. Russotto <russotto@speakeasy.net>
// http://www.speakeasy.org/~russotto/arseniccomp.html

// ported to Go
// Copyright (C) 2025 Elliot Nunn

// This library is free software; you can redistribute it and/or
// modify it under the terms of the GNU Lesser General Public
// License as published by the Free Software Foundation; either
// version 2.1 of the License, or (at your option) any later version.

// This library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
// Lesser General Public License for more details.

// You should have received a copy of the GNU Lesser General Public
// License along with this library; if not, write to the Free Software
// Foundation, Inc., 59 Temple Place, Suite 330, Boston, MA  02111-1307  USA

package sit

import (
	"bufio"
	"fmt"
	"io"
)

type SIT14Buffer struct {
	data uint16
	bits int8
}

type SIT14Data struct {
	br		*bufio.Reader
	bitbuf		uint64 // current bit buffer
	bits		uint16 // number of valid bits in bitbuf
	code		[308]uint8
	codecopy	[308]uint8
	freq		[308]uint16
	buff		[308]uint32

	var1		[52]uint8
	var2		[52]uint16
	var3		[150]uint16 // 75*2
	var4		[76]uint8
	var5		[75]uint32
	var6		[1024]uint8
	var7		[616]uint16 // 308*2
	var8		[0x4000]uint8

	window		[0x40000]uint8
}

func getBitsLow(s *SIT14Data, bits uint8) uint32 {
	for s.bits < uint16(bits) {
		b, err := s.br.ReadByte()
		if err != nil {
			panic("sit14: failed to get byte")
		}
		s.bitbuf |= uint64(b) << s.bits
		s.bits += 8
	}

	mask := uint64((1 << bits) - 1)
	val := uint32(s.bitbuf & mask)

	s.bitbuf >>= bits
	s.bits -= uint16(bits)

	return val
}

func byteBoundary(s *SIT14Data) {
	if s.bits > 0 {
		rem := s.bits % 8
		if rem != 0 {
			getBitsLow(s, uint8(rem))
		}
	}
}

func uint8ToUint16Slice(src []uint8) []uint16 {
    dst := make([]uint16, len(src))
    for i, v := range src {
        dst[i] = uint16(v)
    }
    return dst
}

// code used to be unit8, using uint16 here for now to avoid casting
func SIT14_Update(first uint16, last uint16, code []uint16, freq []uint16) {
	var i, j uint16

	for last-first > 1 {
		i = first
		j = last
		for j > i {
			for i+1 < last && code[first] > code[i+1] {
				i++
			}
			for j-1 > last && code[first] < code[j-1] {
				j--
			}
			if j > i {
				var t uint16
				t = code[i]; code[i] = code[j];	code[j] = t;
				t = freq[i]; freq[i] = freq[j]; freq[j] = t;
			}
		}
		if first != j {
			var t uint16
			t = code[first]; code[first] = code[j]; code[j] = t;
			t = freq[first]; freq[first] = freq[j];	freq[j] = t;
			i = j + 1
			if last-i <= j-first {
				SIT14_Update(i, last, code, freq)
				last = j
			} else {
				SIT14_Update(first, j, code, freq)
				first = i
			}
		} else {
			first++
		}
	}
}

func SIT14_ReadTree(s *SIT14Data, codesize uint16, result []uint16) {
	var size, i, j, k, l, m, n, o uint32

	i = 0
	k = getBitsLow(s, 1)
	j = getBitsLow(s, 2)+2;
	o = getBitsLow(s, 3)+1;
	size = 1<<j;
	m = size-1;
	if k != 0 {
		k = m - 1
	} else {
		// -1 for unisgned int is not allowed in go, underflow manually
		k = ^uint32(0)
	}

	if getBitsLow(s, 2)&1 != 0 {
		SIT14_ReadTree(s, uint16(size), s.freq[:size*2])
		for i < uint32(codesize) {
			l = 0;
			for {
				l = uint32(s.freq[l + getBitsLow(s, 1)])
				n = size << 1
				if n <= l {
					break
				}
			}
			l -= n
			if k != l {
				if l == m {
					l = 0
					for {
						l = uint32(s.freq[l + getBitsLow(s, 1)])
						n = size << 1;
						if n <= l {
							break
						}
					}
					l += 3 - n
					for l > 0 {
						l--
						s.code[i] = s.code[i-1]
						i++
					}
				} else {
					s.code[i] = uint8(l + o)
				}
			} else {
				s.code[i] = 0
			}
			i++
		}
	} else {
		for i < uint32(codesize) {
			l = getBitsLow(s, uint8(j))
			if k != l {
				if l == m {
					l = getBitsLow(s, uint8(j)) + 3
					for l > 0 {
						l--
						s.code[i] = s.code[i-1]
						i++
					}
				} else {
					s.code[i] = uint8(l + o)
					i++
				}
			} else {
				s.code[i] = 0
				i++
			}
		}
	}

	for i = 0; i < uint32(codesize); {
		s.codecopy[i] = s.code[i]
		s.freq[i] = uint16(i)
		i++
	}

	SIT14_Update(0, codesize, uint8ToUint16Slice(s.codecopy[:codesize]), s.freq[:codesize])

	for i = 0; i < uint32(codesize) && s.codecopy[i] == 0; i++ {}
	for j = 0; j < uint32(codesize); {
		if i != 0 {
			j <<= uint32(s.codecopy[i] - s.codecopy[i-1])
		}
		k = uint32(s.codecopy[i])
		m = 0
		for l = j; k > 0; k-- {
			m = (m << 1) | (l&1)
			l >>= 1
		}
		s.buff[s.freq[i]] = m
		i++
		j++
	}

	for i = 0; i < uint32(codesize*2); i++ {
		result[i] = 0
	}
	j = 2
	for i = 0; i < uint32(codesize); i++ {
		l = 0
		m = s.buff[i]

		for k = 0; k < uint32(s.code[i]); k++ {
			l += (m&1)
			if s.code[i]-1 <= uint8(k) {
				result[l] = codesize * 2 + uint16(i)
			} else {
				if result[l] == 0 {
					result[l] = uint16(j)
					j += 2
				}
				l = uint32(result[l])
			}
			m >>= 1
		}
	}
	byteBoundary(s)
}

func sit14(r io.Reader, dstsize uint32) io.ReadCloser {
	pr, pw := io.Pipe()
	go sit14copy(pw, r, dstsize)
	return pr
}

func sit14copy(dst *io.PipeWriter, src io.Reader, dstsize uint32) {
	defer func() {
		if r := recover(); r != nil {
			dst.CloseWithError(fmt.Errorf("internal StuffIt/SIT14 panic: %v", r))
		} else {
			dst.Close()
		}
	}()

	var s = SIT14Data{}
	s.br = bufio.NewReaderSize(src, 4096)
	bw := bufio.NewWriterSize(dst, 4096)
	defer bw.Flush()
	var i, j, k, l, m, n uint32

	for i, k = 0, 0; i < 52; i++ {
		s.var2[i] = uint16(k);
		if i >= 4 {
			s.var1[i] = uint8(i - 4) >> 2
		} else {
			s.var1[i] = 0
		}
		k += 1 << s.var1[i]
	}

	for i = 0; i < 4; i++ {
		s.var8[i] = uint8(i)
	}

	for m, l = 1, 4; i < 0x4000; m <<= 1 {
		for n = l + 4; l < n; l++ {
			for j = 0; j < m; j++ {
				s.var8[i] = uint8(l)
				i++
			}
		}
	}

	for i, k = 0, 1; i < 75; i++ {
		s.var5[i] = k
		if i >= 3 {
			s.var4[i] = uint8(i - 3) >> 2
		} else {
			s.var4[i] = 0
		}
	}

	for i = 0; i < 4; i++ {
		s.var6[i] = uint8(i - 1)
	}

	for m, l = 1, 3; i < 0x400; m <<= 1 {
		for n = l + 4; l < n; l++ {
			for j = 0; j < m; j++ {
				s.var6[i] = uint8(l)
				i++
			}
		}
	}

	m = getBitsLow(&s, 16) // number of blocks
	j = 0 // window position

	for m > 0 {
		_, err := s.br.Peek(1)
		if err != nil {
			break
		}
		m--

		for i = 0; i < 616; {
			i = uint32(s.var7[i + getBitsLow(&s, 1)])
		i -= 616
		}
		if i < 0x100 {
			bw.WriteByte(byte(i))
			s.window[j] = uint8(i)
			j &= 0xFFFF
			n--
			j++
		} else {
			i -= 0x100
			k = uint32(s.var2[i] + 4)
			i = uint32(s.var1[i])
			if i != 0 {
				k += getBitsLow(&s, uint8(i))
			}
			for i = 0; i < 150; {
				i = uint32(s.var3[i + getBitsLow(&s, 1)])
			}
			i -= 150
			l = s.var5[i]
			i = uint32(s.var4[i])
			if i != 0 {
				l += getBitsLow(&s, uint8(i))
			}
			n -= k
			l = j + 0x40000 - l
			for ;k > 0; k-- {
				l &= 0x3FFFF
				bw.WriteByte(s.window[l])
				s.window[j] = s.window[l]
				j++
				l++
			}
		}
		byteBoundary(&s)
	}
}