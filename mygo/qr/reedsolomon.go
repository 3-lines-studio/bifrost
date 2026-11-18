package qr

import "sync"

// reedSolomon returns the error correction codewords of data, as many as
// count: the remainder of data, read as a polynomial, divided by the generator
// polynomial of that count, in the field of GF(256) the specification uses.
func reedSolomon(data []byte, count int) []byte {
	generator := generatorFor(count)
	out := make([]byte, count)
	for _, value := range data {
		// out[0] is the coefficient of the highest power of the remainder.
		term := value ^ out[0]
		copy(out, out[1:])
		out[count-1] = 0
		for i, coefficient := range generator {
			out[i] ^= multiply(coefficient, term)
		}
	}
	return out
}

// field is the multiplication table of GF(256) with the polynomial 0x11D.
var field = sync.OnceValues(func() ([512]byte, [256]int) {
	exp := [512]byte{}
	log := [256]int{}
	value := 1
	for i := 0; i < 255; i++ {
		exp[i] = byte(value)
		log[value] = i
		value <<= 1
		if value&0x100 != 0 {
			value ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		exp[i] = exp[i-255]
	}
	return exp, log
})

// multiply returns the product of two elements of GF(256).
func multiply(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	exp, log := field()
	return exp[log[a]+log[b]]
}

// generators are the generator polynomials, by the number of error correction
// codewords they correct.
var generators sync.Map

// generatorFor returns the coefficients of the generator polynomial of count
// error correction codewords, without its leading one.
func generatorFor(count int) []byte {
	if known, ok := generators.Load(count); ok {
		return known.([]byte)
	}
	polynomial := []byte{1}
	for i := 0; i < count; i++ {
		next := make([]byte, len(polynomial)+1)
		for j, coefficient := range polynomial {
			next[j] ^= coefficient
			next[j+1] ^= multiply(coefficient, power(i))
		}
		polynomial = next
	}
	generator := polynomial[1:]
	generators.Store(count, generator)
	return generator
}

// power returns the i-th power of 2 in GF(256), as the generator polynomial
// takes its roots.
func power(i int) byte {
	exp, _ := field()
	return exp[i]
}
