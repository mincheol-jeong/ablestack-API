package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ablecloud.io/ablestack-api/docs"
	Auth "ablecloud.io/ablestack-api/internal/handler/auth"
	CubeHandler "ablecloud.io/ablestack-api/internal/handler/cube"
	GlueHandler "ablecloud.io/ablestack-api/internal/handler/glue"
	MoldHandler "ablecloud.io/ablestack-api/internal/handler/mold"
	SwaggerHandler "ablecloud.io/ablestack-api/internal/handler/swagger"
	"ablecloud.io/ablestack-api/internal/infra/logging"
	C "ablecloud.io/ablestack-api/internal/service/controller"
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
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Enter the token with the `Bearer ` prefix, e.g. `Bearer eyJ...`
//	@Security					BearerAuth

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
	c.StatusRegister(CubeHandler.UpdateHosts)
	c.StatusRegister(CubeHandler.UpdateClusterConfig)
	// Background daily ssh-keyscan based on cluster.json + systemProfile
	c.StatusRegister(CubeHandler.AutoSSHKnownHostsScan)
	c.StatusRegister(CubeHandler.AutoLegacyPythonCronCleanup)
	c.StatusRegister(CubeHandler.AutoCCVMSnapshotBackup)
	c.StatusRegister(CubeHandler.AutoCCVMDBDumpBackup)
	c.StatusRegister(CubeHandler.AutoCCVMFileBackupSchedule)
	c.StatusRegister(CubeHandler.UpdateNICs)
	c.StatusRegister(CubeHandler.UpdateDisk)

	go c.Start()
	apiPort, err := resolveAPIPort(os.Getenv("ABLESTACK_API_PORT"))
	if err != nil {
		log.Fatalf("invalid ABLESTACK_API_PORT: %v", err)
	}
	docs.SwaggerInfo.Schemes = []string{"http", "https"}
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	//gin.SetMode(gin.DebugMode)
	gin.SetMode(gin.ReleaseMode)
	logging.StartRotationWorker()

	r := gin.New()
	r.ForwardedByClientIP = true
	err = r.SetTrustedProxies(nil)
	if err != nil {
		c.AddError(err)
	}

	r.Use(logging.GinRequestLogger())
	// Recovery 미들웨어는 panic이 발생하면 500 에러를 쓰고 detail log에 stack을 남깁니다.
	r.Use(logging.GinRecovery())
	// CORS (Swagger/웹 클라이언트 대응)
	r.Use(func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		ctx.Writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Cube-Internal-Token")
		if ctx.Request.Method == http.MethodOptions {
			ctx.AbortWithStatus(http.StatusNoContent)
			return
		}
		ctx.Next()
	})
	r.Use(Auth.Middleware())

	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", CubeHandler.Health)
		auth := v1.Group("/auth")
		{
			auth.POST("/login", Auth.Login)
			auth.GET("/me", Auth.Me)
			auth.POST("/internal-token/rotate", Auth.RotateInternalToken)
			auth.POST("/internal-token/apply", Auth.ApplyInternalToken)
		}
		cube := v1.Group("/cube")
		{
			cube.GET("/cluster/health", CubeHandler.ClusterHealth)
			cube.GET("/deploy/status", CubeHandler.GetDeployStatus)
			cube.POST("/deploy/run", CubeHandler.StartDeployRun)
			cube.GET("/deploy/jobs", CubeHandler.ListDeployRunJobs)
			cube.GET("/deploy/jobs/:job_id", CubeHandler.GetDeployRunJob)
			cube.GET("/hosts", CubeHandler.GetHosts)
			cube.POST("/hosts/remove", CubeHandler.StartHostRemove)
			cube.GET("/hosts/remove/jobs", CubeHandler.ListHostRemoveJobs)
			cube.GET("/hosts/remove/jobs/:job_id", CubeHandler.GetHostRemoveJob)
			cube.GET("/test", CubeHandler.GetHosts)
			cube.GET("/cluster/config", CubeHandler.GetClusterConfig)
			cube.POST("/cluster/apply", CubeHandler.ApplyClusterConfig)
			cube.POST("/cluster/apply-local", CubeHandler.ApplyClusterConfigLocal)
			cube.POST("/cloudinit/status", CubeHandler.CloudInitStatus)
			cube.POST("/cloudinit/ccvm/generate", CubeHandler.CreateCCVMCloudInit)
			cube.POST("/cloudinit/scvm/generate", CubeHandler.CreateSCVMCloudInit)
			cube.GET("/url", CubeHandler.GetURL)
			cube.GET("/system/config", CubeHandler.GetSystemConfig)
			cube.POST("/system/config", CubeHandler.UpdateSystemConfig)
			cube.POST("/time-server", CubeHandler.ConfigureTimeServer)
			cube.GET("/ccvm/status", CubeHandler.GetCCVMStatus)
			cube.POST("/ccvm/edit", CubeHandler.EditCCVM)
			cube.POST("/ccvm/create", CubeHandler.CreateCCVM)
			cube.POST("/ccvm/xml", CubeHandler.CreateCCVMXML)
			cube.POST("/ccvm/bootstrap", CubeHandler.CCVMBootstrap)
			cube.POST("/ccvm/snap", CubeHandler.CCVMSnap)
			cube.POST("/ccvm/backup", CubeHandler.CCVMBackup)
			cube.POST("/ccvm/restore", CubeHandler.CCVMRestore)
			cube.POST("/ccvm/lifecycle", CubeHandler.CCVMLifecycle)
			cube.POST("/ccvm/secondary/resize", CubeHandler.CCVMSecondaryResize)
			cube.POST("/auto-shutdown", CubeHandler.AutoShutdown)
			cube.POST("/local/manage", CubeHandler.LocalManage)
			cube.POST("/clvm/manage", CubeHandler.CLVMManage)
			cube.POST("/hba/manage", CubeHandler.HBAManage)
			cube.POST("/multipath/sync", CubeHandler.MultipathSync)
			cube.GET("/gfs/disk/status", CubeHandler.GetGFSDiskStatus)
			cube.GET("/gfs/resource/status", CubeHandler.GetGFSResourceStatus)
			cube.POST("/gfs/manage", CubeHandler.GFSManage)
			cube.POST("/rbd/manage", CubeHandler.RBDManage)
			cube.GET("/gluecluster/status", CubeHandler.GetGlueClusterStatus)
			cube.POST("/gluecluster/update", CubeHandler.UpdateGlueCluster)
			cube.POST("/glue/config/update", CubeHandler.UpdateGlueConfig)
			cube.GET("/scvm/status", CubeHandler.GetSCVMStatus)
			cube.POST("/scvm/xml", CubeHandler.CreateSCVMXML)
			cube.POST("/scvm/bootstrap", CubeHandler.SCVMBootstrap)
			cube.POST("/scvm/lifecycle", CubeHandler.SCVMLifecycle)
			cube.POST("/pcs/control", CubeHandler.CCVMPCSControl)
			cube.POST("/ccvm/service/control", CubeHandler.CCVMServiceControl)
			cube.POST("/ccvm/monitoring/config", CubeHandler.CCVMMonitoringConfig)
			cube.POST("/version/update", CubeHandler.VersionUpdate)
			cube.POST("/security/patch", CubeHandler.SecurityPatch)
			cube.GET("/security/evidence", CubeHandler.GetLatestSecurityEvidence)
			cube.POST("/security/evidence", CubeHandler.GenerateSecurityEvidence)
			cube.GET("/security/evidence/download", CubeHandler.DownloadLatestSecurityEvidence)
			cube.POST("/ssh/key", CubeHandler.SSHKey)
			cube.POST("/ssh/trust", CubeHandler.SSHTrust)
			cube.POST("/license", CubeHandler.LicenseControl)
			cube.POST("/license/apply", CubeHandler.ApplyLicenseToCluster)
			cube.POST("/db/dump", CubeHandler.DBDump)
			cube.GET("/nics", CubeHandler.GetNICs)
			cube.GET("/disk", CubeHandler.GetDisk)
		}
		GlueHandler.RegisterRoutesIfSCVM(v1.Group("/glue"))
		MoldHandler.RegisterRoutesIfCCVM(v1.Group("/mold"))
		v1.GET("/version", CubeHandler.Version)
		v1.GET("/err", c.Error)
		v1.DELETE("/err", c.DeleteError)
		v1.GET("/swagger/*any", SwaggerHandler.Handler())
	}
	// Convenience: allow /swagger and /swagger/index.html without /api/v1 prefix.
	r.GET("/swagger", func(ctx *gin.Context) {
		ctx.Redirect(302, "/swagger/index.html")
	})
	r.GET("/swagger/*any", SwaggerHandler.Handler())
	r.GET("/health", CubeHandler.Health)

	err = r.Run(":" + apiPort)
	if err != nil {
		c.AddError(err)
	}

	c.Stop()
	fmt.Println("end")
}

func resolveAPIPort(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "18090", nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("port must be an integer between 1 and 65535")
	}
	return strconv.Itoa(port), nil
}

func errorMaker() {
	c := C.Init()
	c.AddError(errors.New(time.Now().String()))
}
