package controller

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"ablecloud.io/ablestack-api/internal/infra/logging"
	"ablecloud.io/ablestack-api/internal/infra/utils"
	Cube "ablecloud.io/ablestack-api/internal/model/cube"
)

const controllerHandlerInterval = 30 * time.Second

type TypeController struct {
	Cube *Cube.TypeCUBE `json:"cube"`

	mu              sync.RWMutex
	handlers        []func()
	runningHandlers map[uintptr]struct{}
	errors          utils.Errors
	interval        time.Duration
	workerWG        sync.WaitGroup

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
} //	@name	TypeController

var lockController sync.Once
var controller *TypeController

func Init() *TypeController {
	lockController.Do(func() {
		fmt.Println("Creating ", reflect.TypeOf(controller), " now.")
		controller = &TypeController{
			Cube:            Cube.Cube(),
			runningHandlers: make(map[uintptr]struct{}),
		}
	})
	return controller
}

func (c *TypeController) StatusRegister(fn func()) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	c.handlers = append(c.handlers, fn)
	c.mu.Unlock()
}

// Start runs registered status handlers immediately and then on the configured
// interval. A handler is never started while its previous invocation is active.
func (c *TypeController) Start() {
	c.lifecycleMu.Lock()
	if c.cancel != nil {
		c.lifecycleMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	c.cancel = cancel
	c.done = done
	c.lifecycleMu.Unlock()

	defer func() {
		c.workerWG.Wait()
		c.lifecycleMu.Lock()
		if c.done == done {
			c.cancel = nil
			c.done = nil
		}
		close(done)
		c.lifecycleMu.Unlock()
	}()

	c.dispatchHandlers(ctx)
	ticker := time.NewTicker(c.handlerInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.dispatchHandlers(ctx)
		}
	}
}

func (c *TypeController) handlerInterval() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.interval > 0 {
		return c.interval
	}
	return controllerHandlerInterval
}

func (c *TypeController) dispatchHandlers(ctx context.Context) {
	c.mu.RLock()
	handlers := append([]func(){}, c.handlers...)
	c.mu.RUnlock()

	for _, handler := range handlers {
		select {
		case <-ctx.Done():
			return
		default:
		}
		c.startHandler(handler)
	}
}

func (c *TypeController) startHandler(handler func()) {
	if handler == nil {
		return
	}
	key := reflect.ValueOf(handler).Pointer()
	c.mu.Lock()
	if c.runningHandlers == nil {
		c.runningHandlers = make(map[uintptr]struct{})
	}
	if _, running := c.runningHandlers[key]; running {
		c.mu.Unlock()
		return
	}
	c.runningHandlers[key] = struct{}{}
	c.workerWG.Add(1)
	c.mu.Unlock()

	go func() {
		defer c.workerWG.Done()
		defer func() {
			c.mu.Lock()
			delete(c.runningHandlers, key)
			c.mu.Unlock()
		}()
		runRegisteredHandler(handler)
	}()
}

func runRegisteredHandler(handler func()) {
	job := registeredHandlerName(handler)
	defer func() {
		if recovered := recover(); recovered != nil {
			logging.RecordJobPanic(job, recovered, nil)
		}
	}()
	handler()
}

func registeredHandlerName(handler func()) string {
	if handler == nil {
		return "controller.unknown"
	}
	name := runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

func (c *TypeController) Stop() {
	c.lifecycleMu.Lock()
	cancel := c.cancel
	done := c.done
	c.lifecycleMu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
}

func (c *TypeController) AddError(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	c.errors.Errors = append(c.errors.Errors, utils.Errorlog{Error: err.Error(), Time: time.Now()})
	c.mu.Unlock()
}

func AddError(err error) {
	Init()
	controller.AddError(err)
}

func (c *TypeController) GetError() *utils.Errors {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return &utils.Errors{Errors: append([]utils.Errorlog(nil), c.errors.Errors...)}
}

func (c *TypeController) ClearError() {
	c.mu.Lock()
	c.errors.Errors = nil
	c.mu.Unlock()
}

// Error godoc
//
//	@Summary		Error
//	@Description	Error.
//	@Tags			Cube-Error
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
