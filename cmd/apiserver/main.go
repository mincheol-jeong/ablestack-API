package main

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	CubeModel "github.com/ycyun/Cube-API/internal/domain/model/cube"
	C "github.com/ycyun/Cube-API/internal/service/controller"

	//Cube "github.com/ycyun/Cube-API/internal/api/handler/cube"
	"log"
	"time"

	"github.com/ycyun/Cube-API/docs"
	Dashboard "github.com/ycyun/Cube-API/internal/api/handler/dashboard"
	Glue "github.com/ycyun/Cube-API/internal/api/handler/glue"
	Mold "github.com/ycyun/Cube-API/internal/api/handler/mold"
	PCS "github.com/ycyun/Cube-API/internal/api/handler/pcs"
	UTILS "github.com/ycyun/Cube-API/internal/infra/utils"
)

//	@title			Cube API
//	@version		1.0
//	@description	This is a Cube-API server.
//	@termsOfService	https://ablecloud.io/

//	@contact.name	API Support
//	@contact.url	https://www.ablecloud.io/support
//	@contact.email	ycyun@ablecloud.io

//	@license.name	Apache 2.0
//	@license.url	https://www.apache.org/licenses/LICENSE-2.0.html

//	@ssshost						10.211.55.11:8080
//	@BasePath					/api/v1
//	@Schemes					http https
//	@securityDefinitions.basic	None

// @externalDocs.description	ABLECLOUD
// @externalDocs.url			https://www.ablecloud.io
func main() {
	// 시간대 설정
	location, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		panic(err)
	}
	// Set the timezone for the current process
	time.Local = location

	c := C.Init()
	c.LoadConfig()
	cubeModel := CubeModel.Cube()

	//c.StatusRegister(Mold.MonitorStatus)
	c.StatusRegister(Glue.Monitor)
	//c.StatusRegister(Dashboard.Monitor)
	c.StatusRegister(PCS.Monitor)
	c.StatusRegister(cubeModel.Hosts.Update)
	c.StatusRegister(cubeModel.NICs.Update)
	c.StatusRegister(cubeModel.Disks.Update)
	c.StatusRegister(C.SaveConfig)

	go c.Start()
	APIPort := "8090"
	docs.SwaggerInfo.Schemes = []string{"http", "https"}
	docs.SwaggerInfo.Host = UTILS.GetLocalIP().String() + ":" + APIPort
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	r := gin.Default()
	//gin.SetMode(gin.DebugMode)
	gin.SetMode(gin.ReleaseMode)
	r.ForwardedByClientIP = true
	err = r.SetTrustedProxies(nil)
	if err != nil {
		c.AddError(err)
	}

	r.Use(gin.Logger())

	// Recovery 미들웨어는 panic이 발생하면 500 에러를 씁니다.
	r.Use(gin.Recovery())

	v1 := r.Group("/api/v1")
	{
		v1.GET("/neighbor", c.GetNeighbor)
		v1.GET("/neighbor/info", c.GetNeighborInfo)
		v1.POST("/neighbor", c.PutNeighbor)
		v1.PUT("/neighbor", c.PutNeighbor)
		v1.DELETE("/neighbor", c.DeleteNeighbor)
		cube := v1.Group("/cube")
		{
			cube.GET("/hosts", cubeModel.Hosts.Get)
			cube.GET("/test", cubeModel.Hosts.Get)
			cube.GET("/nics", cubeModel.NICs.Get)
			cube.GET("/disk", cubeModel.Disks.Get)
		}
		glue := v1.Group("/glue")
		{
			glue.GET("/", Glue.GetGlueStatus)
			glue.GET("/auth", Glue.GetGlueAuth)
			glue.GET("/auth/:username", Glue.GetGlueAuth)
			glue.GET("/auths", Glue.GetGlueAuths)
		}
		mold := v1.Group("/mold")
		{
			mold.GET("", Mold.GetStatus)
			mold.GET("/ccvm", Mold.GetCCVMInfo)
		}
		pcs := v1.Group("/pcs")
		{
			pcs.GET("", PCS.GetStatus)
			pcs.GET("/resources", PCS.GetResource)
		}
		dashboard := v1.Group("/dashboard")
		{
			dashboard.GET("", Dashboard.GetStatus)

		}
		//v1.Any("/version", Cube.Version)
		v1.GET("/err", c.Error)
		v1.DELETE("/err", c.DeleteError)
		v1.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}
	// Convenience: allow /swagger and /swagger/index.html without /api/v1 prefix.
	r.GET("/swagger", func(ctx *gin.Context) {
		ctx.Redirect(302, "/swagger/index.html")
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	err = r.Run(":" + APIPort)
	if err != nil {
		c.AddError(err)
	}

	c.Stop()
	fmt.Println("end")
}

func errorMaker() {
	c := C.Init()
	c.AddError(errors.New(time.Now().String()))
}
