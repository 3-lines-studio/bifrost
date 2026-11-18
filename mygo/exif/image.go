package exif

// readJPEG walks the segments of a JPEG file: the APP1 segment carries the
// EXIF of the picture, and a start-of-frame segment its size.
func readJPEG(b []byte) (Metadata, error) {
	var metadata Metadata
	found := false
	for at := 2; at+4 <= len(b); {
		if b[at] != 0xFF {
			return metadata, ErrFormat
		}
		marker := b[at+1]
		if marker == 0xFF {
			at++
			continue
		}
		length := int(b[at+2])<<8 | int(b[at+3])
		if length < 2 || at+2+length > len(b) {
			return metadata, nil
		}
		segment := b[at+4 : at+2+length]
		switch {
		case marker == 0xE1 && len(segment) > 6 && string(segment[:6]) == "Exif\x00\x00":
			parseTIFF(segment[6:], &metadata)
			found = true
		case isStartOfFrame(marker):
			metadata.Height = int(segment[1])<<8 | int(segment[2])
			metadata.Width = int(segment[3])<<8 | int(segment[4])
			found = true
		case marker == 0xDA:
			return metadata, nil
		}
		at += 2 + length
	}
	if !found {
		return metadata, ErrFormat
	}
	return metadata, nil
}

// isStartOfFrame reports whether the marker starts a frame, whose segment
// carries the size of the picture. C4 is a table, C8 an extension and CC an
// arithmetic coding table, which have the marker numbers of frames.
func isStartOfFrame(marker byte) bool {
	return marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC
}

// readPNG reads the size of the picture from its IHDR chunk and the EXIF it may
// carry in an eXIf chunk.
func readPNG(b []byte) (Metadata, error) {
	var metadata Metadata
	for at := 8; at+8 <= len(b); {
		length := int(b[at])<<24 | int(b[at+1])<<16 | int(b[at+2])<<8 | int(b[at+3])
		kind := string(b[at+4 : at+8])
		start := at + 8
		if length < 0 || start+length > len(b) {
			break
		}
		chunk := b[start : start+length]
		switch kind {
		case "IHDR":
			if len(chunk) < 8 {
				return metadata, nil
			}
			metadata.Width = int(chunk[0])<<24 | int(chunk[1])<<16 | int(chunk[2])<<8 | int(chunk[3])
			metadata.Height = int(chunk[4])<<24 | int(chunk[5])<<16 | int(chunk[6])<<8 | int(chunk[7])
		case "eXIf":
			parseTIFF(chunk, &metadata)
			return metadata, nil
		case "IDAT", "IEND":
			return metadata, nil
		}
		at = start + length + 4
	}
	return metadata, nil
}

// readWebP reads the size of the picture of the headers of a WebP file.
func readWebP(b []byte) (Metadata, error) {
	var metadata Metadata
	kind := string(b[12:16])
	switch kind {
	case "VP8X":
		if len(b) < 30 {
			return metadata, nil
		}
		metadata.Width = 1 + int(b[24]) + int(b[25])<<8 + int(b[26])<<16
		metadata.Height = 1 + int(b[27]) + int(b[28])<<8 + int(b[29])<<16
	case "VP8 ":
		if len(b) < 30 {
			return metadata, nil
		}
		metadata.Width = int(b[26]) | int(b[27])<<8
		metadata.Height = int(b[28]) | int(b[29])<<8
	case "VP8L":
		if len(b) < 25 {
			return metadata, nil
		}
		bits := int(b[21]) | int(b[22])<<8 | int(b[23])<<16 | int(b[24])<<24
		metadata.Width = (bits & 0x3FFF) + 1
		metadata.Height = (bits>>14)&0x3FFF + 1
	default:
		return metadata, ErrFormat
	}
	return metadata, nil
}
