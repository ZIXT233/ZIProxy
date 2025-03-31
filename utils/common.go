package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"reflect"
)

func MergeStruct(dst, src interface{}) {
	dstValue := reflect.ValueOf(dst).Elem() // 获取 dst 的可修改值
	srcValue := reflect.ValueOf(src).Elem() // 获取 src 的值

	// 遍历 src 的所有字段
	for i := 0; i < srcValue.NumField(); i++ {
		srcField := srcValue.Field(i) // src 的字段值
		dstField := dstValue.Field(i) // dst 的对应字段值

		// 检查 src 字段是否为 nil
		if srcField.Kind() == reflect.Ptr && !srcField.IsNil() {
			// 如果 src 字段非 nil，则赋值到 dst
			dstField.Set(srcField)
		} else if srcField.Kind() != reflect.Ptr && srcField.IsValid() && !isEmptyValue(srcField) {
			// 如果 src 字段是非指针类型且非空，则赋值到 dst
			dstField.Set(srcField)
		}
	}
}

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	default:
		return false
	}
}

func GenerateBase64RandomString(length int) (string, error) {
	// 计算需要的随机字节数
	randomBytes := make([]byte, (length*6+7)/8) // Base64 每 3 字节生成 4 字符
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	// 编码为 Base64 并截取指定长度
	base64Str := base64.RawURLEncoding.EncodeToString(randomBytes)
	return base64Str[:length], nil
}

func CryptoRandomInRange(min, max int) (int, error) {
	if min > max {
		return 0, fmt.Errorf("invalid range: min (%d) cannot be greater than max (%d)", min, max)
	}
	rangeSize := big.NewInt(int64(max - min + 1))
	randomNum, err := rand.Int(rand.Reader, rangeSize) // 生成 [0, rangeSize) 的随机数
	if err != nil {
		return 0, err
	}
	return int(randomNum.Int64()) + min, nil // 将随机数映射到 [min, max]
}

func ReadUtil(reader io.Reader, end byte) ([]byte, error) {
	buf := make([]byte, 0, 1024)
	c := make([]byte, 1)
	for {
		_, err := reader.Read(c)
		if err != nil {
			return buf, err
		}
		buf = append(buf, c...)
		if c[0] == end {
			break
		}
	}
	return buf, nil
}
