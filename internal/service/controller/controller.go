package controller

import (
	"bufio"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	Glue "github.com/ycyun/Cube-API/internal/domain/model/glue"
	Mold "github.com/ycyun/Cube-API/internal/domain/model/mold"
	"github.com/ycyun/Cube-API/internal/infra/utils"
	"io"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"

	Cube "github.com/ycyun/Cube-API/internal/domain/model/cube"
)

type TypeNeighbor struct {
	IP       string `json:"ip"`
	HostName string `json:"hostname"`
}

type TypeNeighbors struct {
	Neighbors []TypeNeighbor `json:"neighbors"`
}

type TypeController struct {
	Handlers []func()             `json:"handlers"`
	running  bool                 `json:"running"`
	Neighbor *TypeNeighbors       `json:"neighbors"`
	errors   *utils.Errors        `json:"errors"`
	version  *utils.TypeVersion   `json:"version"`
	Cube     *Cube.TypeCUBE       `json:"cube"`
	Mold     *Mold.TypeMoldStatus `json:"mold_status"`
	Glue     *Glue.TypeGlueStatus `json:"glue_status"`
} //	@name	TypeController

var lockController sync.Once
var controller *TypeController

func Init() *TypeController {
	if controller == nil {
		lockController.Do(
			func() {
				fmt.Println("Creating ", reflect.TypeOf(controller), " now.")
				controller = &TypeController{}
				controller.Neighbor = &TypeNeighbors{}
				controller.Cube = Cube.Cube()
				controller.Mold = Mold.Status()
				controller.Glue = Glue.Status()
			})
	} else {
		fmt.Println("get old ", reflect.TypeOf(controller), " instance.")
	}
	return controller
}

func (c *TypeController) StatusRegister(fn func()) {

	c.Handlers = append(c.Handlers, fn)
}

func (c *TypeController) Start() {
	c.running = true
	for c.running {
		for _, handler := range c.Handlers {
			go handler()
		}

		time.Sleep(time.Duration(10000) * time.Millisecond)
	}
}

func (c *TypeController) Stop() {
	c.running = false
}

func (c *TypeController) AddError(err error) {
	serr := err.Error()
	c.errors.Errors = append(c.errors.Errors, utils.Errorlog{Error: serr, Time: time.Now()})
}

func AddError(err error) {
	Init()
	controller.AddError(err)
}

func (c *TypeController) GetError() *utils.Errors {
	return c.errors
}

func (c *TypeController) ClearError() {
	c.errors = &utils.Errors{}
}

// Error godoc
//
//	@Summary		Error
//	@Description	Error.
//	@Tags			API, CUBE
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Success		200	{object}	utils.Errorlog
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/err [get]
func (c *TypeController) Error(ctx *gin.Context) {
	ctx.IndentedJSON(http.StatusOK, c.GetError())
}

func (c *TypeController) DeleteError(context *gin.Context) {
	c.ClearError()
	context.IndentedJSON(http.StatusOK, c.GetError())
}

func (c *TypeController) UpdateCCVMInfo() TypeNeighborInfos {
	ret := TypeNeighborInfos{Neighbors: make(map[string]TypeNeighborInfo)}
	for _, neighbor := range c.Neighbor.Neighbors {
		str, code := neighbor.GetFromNeighbor("v1/mold/ccvm")
		ret.Neighbors[neighbor.HostName] = TypeNeighborInfo{str, code}
	}
	return ret
}

func (c *TypeController) LoadConfig() {
	fc, err := os.OpenFile(configPath(), os.O_RDONLY, 0666)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer fc.Close()

	var strconfig []byte
	var config Config

	strconfig, err = io.ReadAll(bufio.NewReader(fc))
	if err != nil {
		fmt.Println(err)
		return
	}

	err = json.Unmarshal(strconfig, &config)

	if err != nil {
		fmt.Println(err)
		return
	}
	//Init()
	c.Neighbor.Neighbors = config.Neighbor
}
