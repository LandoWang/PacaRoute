package pacaroute

import (
	"os"

	"github.com/LandoWang/pacaroute/internal/pacaroute/atomicfile"
)

func atomicWrite(path string, b []byte, mode os.FileMode) error {
	return atomicfile.Write(path, b, mode)
}
