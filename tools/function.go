package tools

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	randNew "math/rand"
	"os"
	"strings"
	"time"
)

// Md5 md5加密
func Md5(src string) string {
	m := md5.New()
	m.Write([]byte(src))
	res := hex.EncodeToString(m.Sum(nil))
	return res
}

// RandomNumberString 获取随机数字
func RandomNumberString(len int) string {
	var numbers = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
	var container string
	length := bytes.NewReader(numbers).Len()
	for i := 1; i <= len; i++ {
		random, err := rand.Int(rand.Reader, big.NewInt(int64(length)))
		if err != nil {
			continue
		}
		container += fmt.Sprintf("%d", numbers[random.Int64()])
	}
	return container
}

// RandNumSimple 取两个数字之间的随机数
func RandNumSimple(min, max int) int {
	if min >= max {
		return min
	}
	return randNew.Intn(max-min+1) + min
}

// CreateARandomString 创建随机字符串
func CreateARandomString(len int) string {
	var container string
	var str = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
	b := bytes.NewBufferString(str)
	length := b.Len()
	bigInt := big.NewInt(int64(length))
	for i := 0; i < len; i++ {
		randomInt, _ := rand.Int(rand.Reader, bigInt)
		container += string(str[randomInt.Int64()])
	}
	return container
}

// SlicePage 计算数组分页
func SlicePage(page, pageSize, nums int64) (sliceStart, sliceEnd int64) {
	if page <= 0 {
		page = 1
	}
	if pageSize < 0 {
		pageSize = 50
	}
	pageCount := int64(math.Ceil(float64(nums) / float64(pageSize)))
	if page > pageCount {
		return 0, 0
	}
	sliceStart = (page - 1) * pageSize
	sliceEnd = sliceStart + pageSize
	if sliceEnd > nums {
		sliceEnd = nums
	}
	return sliceStart, sliceEnd
}

// AesEncrypt AES加密
func AesEncrypt(data, sKey string) (string, error) {
	resData := []byte(data)
	key := []byte(sKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	blockSize := block.BlockSize()
	padding := blockSize - len(resData)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	encryptBytes := append(resData, padText...)
	crypted := make([]byte, len(encryptBytes))
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	blockMode.CryptBlocks(crypted, encryptBytes)
	res := base64.StdEncoding.EncodeToString(crypted)
	return res, nil
}

// AesDecrypt AES解密
func AesDecrypt(data, sKey string) (string, error) {
	resData, _ := base64.StdEncoding.DecodeString(data)
	key := []byte(sKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
	crypted := make([]byte, len(resData))
	blockMode.CryptBlocks(crypted, resData)
	length := len(crypted)
	if length == 0 {
		return "", errors.New("加密字符串错误！")
	}
	unPadding := int(crypted[length-1])
	crypted = crypted[:(length - unPadding)]
	return string(crypted), nil
}

// StrToUnix 时间字符串转time.Time
func StrToUnix(timeStr, layout string) time.Time {
	local, _ := time.LoadLocation("Asia/Shanghai")
	tt, _ := time.ParseInLocation(layout, timeStr, local)
	return tt
}

// IsFileExist 判断文件是否存在
func IsFileExist(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return true
}

// CalculateTimeDifference 计算两个时间之间的秒数
func CalculateTimeDifference(t1Str, t2Str string) int {
	layout := "2006-01-02 15:04:05"
	t1, err := time.Parse(layout, t1Str)
	if err != nil {
		return 0
	}
	t2, _ := time.Parse(layout, t2Str)
	diff := t2.Sub(t1)
	return int(diff.Seconds())
}

// SplitIds 将逗号分隔的 ids 拆为切片，供 GORM in (?) 参数化展开（避免字符串拼接 SQL）
func SplitIds(ids string) []interface{} {
	parts := strings.Split(ids, ",")
	out := make([]interface{}, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
