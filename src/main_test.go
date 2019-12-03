package main

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

var clients []clientData

func Test(t *testing.T) {
	type args struct {
		p             golParams
		expectedAlive []cell
	}
	tests := []struct {
		name string
		args args
	}{
		{"16x16x2-0", args{
			p: golParams{
				turns:       0,
				threads:     2,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 4, y: 5},
				{x: 5, y: 6},
				{x: 3, y: 7},
				{x: 4, y: 7},
				{x: 5, y: 7},
			},
		}},

		{"16x16x4-0", args{
			p: golParams{
				turns:       0,
				threads:     4,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 4, y: 5},
				{x: 5, y: 6},
				{x: 3, y: 7},
				{x: 4, y: 7},
				{x: 5, y: 7},
			},
		}},

		{"16x16x8-0", args{
			p: golParams{
				turns:       0,
				threads:     8,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 4, y: 5},
				{x: 5, y: 6},
				{x: 3, y: 7},
				{x: 4, y: 7},
				{x: 5, y: 7},
			},
		}},

		{"16x16x2-1", args{
			p: golParams{
				turns:       1,
				threads:     2,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 3, y: 6},
				{x: 5, y: 6},
				{x: 4, y: 7},
				{x: 5, y: 7},
				{x: 4, y: 8},
			},
		}},

		{"16x16x4-1", args{
			p: golParams{
				turns:       1,
				threads:     4,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 3, y: 6},
				{x: 5, y: 6},
				{x: 4, y: 7},
				{x: 5, y: 7},
				{x: 4, y: 8},
			},
		}},

		{"16x16x8-1", args{
			p: golParams{
				turns:       1,
				threads:     8,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 3, y: 6},
				{x: 5, y: 6},
				{x: 4, y: 7},
				{x: 5, y: 7},
				{x: 4, y: 8},
			},
		}},

		{"16x16x2-100", args{
			p: golParams{
				turns:       100,
				threads:     2,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 12, y: 0},
				{x: 13, y: 0},
				{x: 14, y: 0},
				{x: 13, y: 14},
				{x: 14, y: 15},
			},
		}},

		{"16x16x4-100", args{
			p: golParams{
				turns:       100,
				threads:     4,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 12, y: 0},
				{x: 13, y: 0},
				{x: 14, y: 0},
				{x: 13, y: 14},
				{x: 14, y: 15},
			},
		}},

		{"16x16x8-100", args{
			p: golParams{
				turns:       100,
				threads:     8,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 12, y: 0},
				{x: 13, y: 0},
				{x: 14, y: 0},
				{x: 13, y: 14},
				{x: 14, y: 15},
			},
		}},

		// New 6, 10, 12 thread tests
		{"16x16x6-1", args{
			p: golParams{
				turns:       1,
				threads:     6,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 3, y: 6},
				{x: 5, y: 6},
				{x: 4, y: 7},
				{x: 5, y: 7},
				{x: 4, y: 8},
			},
		}},

		{"16x16x10-1", args{
			p: golParams{
				turns:       1,
				threads:     10,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 3, y: 6},
				{x: 5, y: 6},
				{x: 4, y: 7},
				{x: 5, y: 7},
				{x: 4, y: 8},
			},
		}},

		{"16x16x12-1", args{
			p: golParams{
				turns:       1,
				threads:     12,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 3, y: 6},
				{x: 5, y: 6},
				{x: 4, y: 7},
				{x: 5, y: 7},
				{x: 4, y: 8},
			},
		}},

		{"16x16x6-100", args{
			p: golParams{
				turns:       100,
				threads:     6,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 12, y: 0},
				{x: 13, y: 0},
				{x: 14, y: 0},
				{x: 13, y: 14},
				{x: 14, y: 15},
			},
		}},

		{"16x16x10-100", args{
			p: golParams{
				turns:       100,
				threads:     10,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 12, y: 0},
				{x: 13, y: 0},
				{x: 14, y: 0},
				{x: 13, y: 14},
				{x: 14, y: 15},
			},
		}},

		{"16x16x12-100", args{
			p: golParams{
				turns:       100,
				threads:     12,
				imageWidth:  16,
				imageHeight: 16,
			},
			expectedAlive: []cell{
				{x: 12, y: 0},
				{x: 13, y: 0},
				{x: 14, y: 0},
				{x: 13, y: 14},
				{x: 14, y: 15},
			},
		}},

		// Special test to be used to generate traces - not a real test
		//{"trace", args{
		//	p: golParams{
		//		turns:       10,
		//		threads:     4,
		//		imageWidth:  64,
		//		imageHeight: 64,
		//	},
		//}},
	}

	start := time.Now()
	// Networking
	fmt.Println("Waiting for", clientNumber, "clients to connect.")
	clients = processClients(clientNumber)
	fmt.Println("Waited for", time.Since(start))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			alive := gameOfLife(test.args.p, nil, clientNumber, clients)
			//fmt.Println("Ran test:", test.name)
			if test.name != "trace" {
				assert.ElementsMatch(t, alive, test.args.expectedAlive)
			}
		})
	}
}

const benchLength = 1000

func Benchmark(b *testing.B) {
	benchmarks := []struct {
		name string
		p    golParams
	}{
		/*{
			"16x16x2", golParams{
				turns:       benchLength,
				threads:     2,
				imageWidth:  16,
				imageHeight: 16,
			}},

		{
			"16x16x4", golParams{
				turns:       benchLength,
				threads:     4,
				imageWidth:  16,
				imageHeight: 16,
			}},

		{
			"16x16x8", golParams{
				turns:       benchLength,
				threads:     8,
				imageWidth:  16,
				imageHeight: 16,
			}},

		{
			"64x64x2", golParams{
				turns:       benchLength,
				threads:     2,
				imageWidth:  64,
				imageHeight: 64,
			}},

		{
			"64x64x4", golParams{
				turns:       benchLength,
				threads:     4,
				imageWidth:  64,
				imageHeight: 64,
			}},

		{
			"64x64x8", golParams{
				turns:       benchLength,
				threads:     8,
				imageWidth:  64,
				imageHeight: 64,
			}},

		{
			"128x128x2", golParams{
				turns:       benchLength,
				threads:     2,
				imageWidth:  128,
				imageHeight: 128,
			}},

		{
			"128x128x4", golParams{
				turns:       benchLength,
				threads:     4,
				imageWidth:  128,
				imageHeight: 128,
			}},

		{
			"128x128x8", golParams{
				turns:       benchLength,
				threads:     8,
				imageWidth:  128,
				imageHeight: 128,
			}},

		{
			"256x256x2", golParams{
				turns:       benchLength,
				threads:     2,
				imageWidth:  256,
				imageHeight: 256,
			}},

		{
			"256x256x4", golParams{
				turns:       benchLength,
				threads:     4,
				imageWidth:  256,
				imageHeight: 256,
			}},

		{
			"256x256x8", golParams{
				turns:       benchLength,
				threads:     8,
				imageWidth:  256,
				imageHeight: 256,
			}},

		{
			"512x512x2", golParams{
				turns:       benchLength,
				threads:     2,
				imageWidth:  512,
				imageHeight: 512,
			}},

		{
			"512x512x4", golParams{
				turns:       benchLength,
				threads:     4,
				imageWidth:  512,
				imageHeight: 512,
			}},

		{
			"512x512x8", golParams{
				turns:       benchLength,
				threads:     8,
				imageWidth:  512,
				imageHeight: 512,
			}},

		*/
		/*{
			"512x512x12", golParams{
				turns:       benchLength,
				threads:     12,
				imageWidth:  512,
				imageHeight: 512,
			}},

		{
			"512x512x16", golParams{
				turns:       benchLength,
				threads:     16,
				imageWidth:  512,
				imageHeight: 512,
			}},

		{
			"512x512x32", golParams{
				turns:       benchLength,
				threads:     32,
				imageWidth:  512,
				imageHeight: 512,
			}},

		{
			"512x512x64", golParams{
				turns:       benchLength,
				threads:     64,
				imageWidth:  512,
				imageHeight: 512,
			}},

		{
			"512x512x96", golParams{
				turns:       benchLength,
				threads:     96,
				imageWidth:  512,
				imageHeight: 512,
			}},

		{
			"512x512x128", golParams{
				turns:       benchLength,
				threads:     128,
				imageWidth:  512,
				imageHeight: 512,
			}},*/
		{
			"5120x5120x32", golParams{
				turns:       100,
				threads:     32,
				imageWidth:  5120,
				imageHeight: 5120,
			}},
		{
			"5120x5120x64", golParams{
				turns:       100,
				threads:     64,
				imageWidth:  5120,
				imageHeight: 5120,
			}},

		{
			"5120x5120x128", golParams{
				turns:       100,
				threads:     128,
				imageWidth:  5120,
				imageHeight: 5120,
			}},
	}

	for _, bm := range benchmarks {
		os.Stdout = nil // Disable all program output apart from benchmark results
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				gameOfLife(bm.p, nil, clientNumber, clients)
				//fmt.Println("Ran bench:", bm.name)
			}
		})
	}
}
