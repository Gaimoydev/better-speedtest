package nodes

import (
	"crypto/des"
	"encoding/hex"
	"errors"
)

func decodeHexDES(key, hexData []byte) ([]byte, error) {
	raw := make([]byte, hex.DecodedLen(len(hexData)))
	n, err := hex.Decode(raw, hexData)
	if err != nil {
		return nil, err
	}
	return desECBDecrypt(key, raw[:n])
}

func desECBDecrypt(key, ct []byte) ([]byte, error) {
	block, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}
	bs := block.BlockSize()
	if len(ct) == 0 || len(ct)%bs != 0 {
		return nil, errors.New("des: ciphertext not a multiple of block size")
	}
	out := make([]byte, len(ct))
	for i := 0; i < len(ct); i += bs {
		block.Decrypt(out[i:i+bs], ct[i:i+bs])
	}
	return pkcs5Unpad(out, bs)
}

func pkcs5Unpad(b []byte, bs int) ([]byte, error) {
	n := len(b)
	if n == 0 || n%bs != 0 {
		return nil, errors.New("pkcs5: bad length")
	}
	pad := int(b[n-1])
	if pad == 0 || pad > bs || pad > n {
		return nil, errors.New("pkcs5: bad padding")
	}
	return b[:n-pad], nil
}
