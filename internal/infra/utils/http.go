package utils

import (
	"log"
	"net"
	"time"
)

func GetLocalIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddress := conn.LocalAddr().(*net.UDPAddr)

	return localAddress.IP
}

// HTTPLogin
// @description
type HTTPLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
} //@name HTTPLogin

// HTTPError
// @description
type HTTPError struct {
	ErrCode int    `json:"err_code"`
	Message string `json:"message"`
} //@name HTTPError

// HTTP400BadRequest
// @description
type HTTP400BadRequest struct {
	HTTPError
	ErrCode int    `json:"err_code" example:"400"`
	Message string `json:"message" example:"status bad request"`
} //@name HTTP400BadRequest

// HTTP404NotFound
// @description
type HTTP404NotFound struct {
	HTTPError
	ErrCode int    `json:"err_code" example:"404"`
	Message string `json:"message" example:"Not Found"`
} //@name HTTP404NotFound

// HTTP500InternalServerError
// @description
type HTTP500InternalServerError struct {
	HTTPError
	ErrCode int    `json:"err_code" example:"500"`
	Message string `json:"message" example:"InternalServerError"`
} //@name HTTP500InternalServerError

type TestReturn struct {
	HTTPError
	Now         time.Time `json:"Time"`
	RefreshTime time.Time `json:"refresh_time"`
}
