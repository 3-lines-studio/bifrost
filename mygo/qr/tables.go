package qr

// specification is what a version of a code holds: its size in modules, its
// codewords, the bits after them that the code leaves light, and how much of it
// the error correction of each level takes, in the blocks the text splits into:
// two groups whose text codewords differ by one. The numbers are the ones of
// the specification, as qrencode's qrspec.c tabulates them.
type specification struct {
	size      int
	words     int
	remainder int
	ec        [4]int
	blocks    [4][2]int
}

// specs are the forty versions of the specification, from 1 to 40.
var specs = [40]specification{
	{size: 21, words: 26, remainder: 0, ec: [4]int{7, 10, 13, 17}, blocks: [4][2]int{{1, 0}, {1, 0}, {1, 0}, {1, 0}}},                   // 1
	{size: 25, words: 44, remainder: 7, ec: [4]int{10, 16, 22, 28}, blocks: [4][2]int{{1, 0}, {1, 0}, {1, 0}, {1, 0}}},                  // 2
	{size: 29, words: 70, remainder: 7, ec: [4]int{15, 26, 36, 44}, blocks: [4][2]int{{1, 0}, {1, 0}, {2, 0}, {2, 0}}},                  // 3
	{size: 33, words: 100, remainder: 7, ec: [4]int{20, 36, 52, 64}, blocks: [4][2]int{{1, 0}, {2, 0}, {2, 0}, {4, 0}}},                 // 4
	{size: 37, words: 134, remainder: 7, ec: [4]int{26, 48, 72, 88}, blocks: [4][2]int{{1, 0}, {2, 0}, {2, 2}, {2, 2}}},                 // 5
	{size: 41, words: 172, remainder: 7, ec: [4]int{36, 64, 96, 112}, blocks: [4][2]int{{2, 0}, {4, 0}, {4, 0}, {4, 0}}},                // 6
	{size: 45, words: 196, remainder: 0, ec: [4]int{40, 72, 108, 130}, blocks: [4][2]int{{2, 0}, {4, 0}, {2, 4}, {4, 1}}},               // 7
	{size: 49, words: 242, remainder: 0, ec: [4]int{48, 88, 132, 156}, blocks: [4][2]int{{2, 0}, {2, 2}, {4, 2}, {4, 2}}},               // 8
	{size: 53, words: 292, remainder: 0, ec: [4]int{60, 110, 160, 192}, blocks: [4][2]int{{2, 0}, {3, 2}, {4, 4}, {4, 4}}},              // 9
	{size: 57, words: 346, remainder: 0, ec: [4]int{72, 130, 192, 224}, blocks: [4][2]int{{2, 2}, {4, 1}, {6, 2}, {6, 2}}},              // 10
	{size: 61, words: 404, remainder: 0, ec: [4]int{80, 150, 224, 264}, blocks: [4][2]int{{4, 0}, {1, 4}, {4, 4}, {3, 8}}},              // 11
	{size: 65, words: 466, remainder: 0, ec: [4]int{96, 176, 260, 308}, blocks: [4][2]int{{2, 2}, {6, 2}, {4, 6}, {7, 4}}},              // 12
	{size: 69, words: 532, remainder: 0, ec: [4]int{104, 198, 288, 352}, blocks: [4][2]int{{4, 0}, {8, 1}, {8, 4}, {12, 4}}},            // 13
	{size: 73, words: 581, remainder: 3, ec: [4]int{120, 216, 320, 384}, blocks: [4][2]int{{3, 1}, {4, 5}, {11, 5}, {11, 5}}},           // 14
	{size: 77, words: 655, remainder: 3, ec: [4]int{132, 240, 360, 432}, blocks: [4][2]int{{5, 1}, {5, 5}, {5, 7}, {11, 7}}},            // 15
	{size: 81, words: 733, remainder: 3, ec: [4]int{144, 280, 408, 480}, blocks: [4][2]int{{5, 1}, {7, 3}, {15, 2}, {3, 13}}},           // 16
	{size: 85, words: 815, remainder: 3, ec: [4]int{168, 308, 448, 532}, blocks: [4][2]int{{1, 5}, {10, 1}, {1, 15}, {2, 17}}},          // 17
	{size: 89, words: 901, remainder: 3, ec: [4]int{180, 338, 504, 588}, blocks: [4][2]int{{5, 1}, {9, 4}, {17, 1}, {2, 19}}},           // 18
	{size: 93, words: 991, remainder: 3, ec: [4]int{196, 364, 546, 650}, blocks: [4][2]int{{3, 4}, {3, 11}, {17, 4}, {9, 16}}},          // 19
	{size: 97, words: 1085, remainder: 3, ec: [4]int{224, 416, 600, 700}, blocks: [4][2]int{{3, 5}, {3, 13}, {15, 5}, {15, 10}}},        // 20
	{size: 101, words: 1156, remainder: 4, ec: [4]int{224, 442, 644, 750}, blocks: [4][2]int{{4, 4}, {17, 0}, {17, 6}, {19, 6}}},        // 21
	{size: 105, words: 1258, remainder: 4, ec: [4]int{252, 476, 690, 816}, blocks: [4][2]int{{2, 7}, {17, 0}, {7, 16}, {34, 0}}},        // 22
	{size: 109, words: 1364, remainder: 4, ec: [4]int{270, 504, 750, 900}, blocks: [4][2]int{{4, 5}, {4, 14}, {11, 14}, {16, 14}}},      // 23
	{size: 113, words: 1474, remainder: 4, ec: [4]int{300, 560, 810, 960}, blocks: [4][2]int{{6, 4}, {6, 14}, {11, 16}, {30, 2}}},       // 24
	{size: 117, words: 1588, remainder: 4, ec: [4]int{312, 588, 870, 1050}, blocks: [4][2]int{{8, 4}, {8, 13}, {7, 22}, {22, 13}}},      // 25
	{size: 121, words: 1706, remainder: 4, ec: [4]int{336, 644, 952, 1110}, blocks: [4][2]int{{10, 2}, {19, 4}, {28, 6}, {33, 4}}},      // 26
	{size: 125, words: 1828, remainder: 4, ec: [4]int{360, 700, 1020, 1200}, blocks: [4][2]int{{8, 4}, {22, 3}, {8, 26}, {12, 28}}},     // 27
	{size: 129, words: 1921, remainder: 3, ec: [4]int{390, 728, 1050, 1260}, blocks: [4][2]int{{3, 10}, {3, 23}, {4, 31}, {11, 31}}},    // 28
	{size: 133, words: 2051, remainder: 3, ec: [4]int{420, 784, 1140, 1350}, blocks: [4][2]int{{7, 7}, {21, 7}, {1, 37}, {19, 26}}},     // 29
	{size: 137, words: 2185, remainder: 3, ec: [4]int{450, 812, 1200, 1440}, blocks: [4][2]int{{5, 10}, {19, 10}, {15, 25}, {23, 25}}},  // 30
	{size: 141, words: 2323, remainder: 3, ec: [4]int{480, 868, 1290, 1530}, blocks: [4][2]int{{13, 3}, {2, 29}, {42, 1}, {23, 28}}},    // 31
	{size: 145, words: 2465, remainder: 3, ec: [4]int{510, 924, 1350, 1620}, blocks: [4][2]int{{17, 0}, {10, 23}, {10, 35}, {19, 35}}},  // 32
	{size: 149, words: 2611, remainder: 3, ec: [4]int{540, 980, 1440, 1710}, blocks: [4][2]int{{17, 1}, {14, 21}, {29, 19}, {11, 46}}},  // 33
	{size: 153, words: 2761, remainder: 3, ec: [4]int{570, 1036, 1530, 1800}, blocks: [4][2]int{{13, 6}, {14, 23}, {44, 7}, {59, 1}}},   // 34
	{size: 157, words: 2876, remainder: 0, ec: [4]int{570, 1064, 1590, 1890}, blocks: [4][2]int{{12, 7}, {12, 26}, {39, 14}, {22, 41}}}, // 35
	{size: 161, words: 3034, remainder: 0, ec: [4]int{600, 1120, 1680, 1980}, blocks: [4][2]int{{6, 14}, {6, 34}, {46, 10}, {2, 64}}},   // 36
	{size: 165, words: 3196, remainder: 0, ec: [4]int{630, 1204, 1770, 2100}, blocks: [4][2]int{{17, 4}, {29, 14}, {49, 10}, {24, 46}}}, // 37
	{size: 169, words: 3362, remainder: 0, ec: [4]int{660, 1260, 1860, 2220}, blocks: [4][2]int{{4, 18}, {13, 32}, {48, 14}, {42, 32}}}, // 38
	{size: 173, words: 3532, remainder: 0, ec: [4]int{720, 1316, 1950, 2310}, blocks: [4][2]int{{20, 4}, {40, 7}, {43, 22}, {10, 67}}},  // 39
	{size: 177, words: 3706, remainder: 0, ec: [4]int{750, 1372, 2040, 2430}, blocks: [4][2]int{{19, 6}, {18, 31}, {34, 34}, {20, 61}}}, // 40
}
