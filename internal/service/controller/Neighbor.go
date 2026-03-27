package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"io"
	"log"
	"net/http"
	"slices"
)

// GetNeighbor godoc
//
//	@Summary		GetNeighbor
//	@Description	GetNeighbor.
//	@Tags			API, CUBE
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Success		200	{object}	TypeNeighbors
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/neighbor [get]
func (c *TypeController) GetNeighbor(context *gin.Context) {
	ret := c.Neighbor
	context.IndentedJSON(http.StatusOK, ret)
}

// PutNeighbor godoc
//
//	@Summary		PutNeighbor
//	@Description	PutNeighbor.
//	@Tags			API, CUBE
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Param			ip			formData	string	true	"Neighbor IP"
//	@Param			hostname	formData	string	true	"Neighbor Hostname"
//	@Success		200			{object}	TypeNeighbors
//	@Failure		400			{object}	HTTP400BadRequest
//	@Failure		404			{object}	HTTP404NotFound
//	@Failure		500			{object}	HTTP500InternalServerError
//	@Router			/neighbor [post]
//	@Router			/neighbor [put]
func (c *TypeController) PutNeighbor(ctx *gin.Context) {
	var neighbor TypeNeighbor

	neighbor.IP = ctx.PostForm("ip")
	neighbor.HostName = ctx.PostForm("hostname")
	if err := ctx.ShouldBindQuery(&neighbor); err != nil {
		log.Println("err: ", err)
	}
	if err := ctx.ShouldBindUri(&neighbor); err != nil {
		log.Println("err: ", err)
	}
	if err := ctx.ShouldBindJSON(&neighbor); err != nil {
		log.Println("err: ", err)
	}

	log.Printf("user: %+v", neighbor)
	c.Neighbor.Neighbors = append(c.Neighbor.Neighbors, neighbor)
	SaveConfig()
	ctx.IndentedJSON(http.StatusOK, c.Neighbor)
}

// DeleteNeighbor godoc
//
//	@Summary		DeleteNeighbor
//	@Description	DeleteNeighbor.
//	@Tags			API, CUBE
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Param			ip			formData	string	true	"Neighbor IP"
//	@Param			hostname	formData	string	true	"Neighbor Hostname"
//	@Success		200			{object}	TypeNeighbor
//	@Failure		400			{object}	HTTP400BadRequest
//	@Failure		404			{object}	HTTP404NotFound
//	@Failure		500			{object}	HTTP500InternalServerError
//	@Router			/neighbor [delete]
func (c *TypeController) DeleteNeighbor(ctx *gin.Context) {
	var neighbor TypeNeighbor

	neighbor.IP = ctx.PostForm("ip")
	neighbor.HostName = ctx.PostForm("hostname")
	if err := ctx.ShouldBindQuery(&neighbor); err != nil {
		log.Println("err: ", err)
	}
	if err := ctx.ShouldBindUri(&neighbor); err != nil {
		log.Println("err: ", err)
	}
	if err := ctx.ShouldBindJSON(&neighbor); err != nil {
		log.Println("err: ", err)
	}

	log.Printf("neighbor: %+v", neighbor)
	if neighbor.IP != "" && neighbor.HostName != "" {

		for i, n := range c.Neighbor.Neighbors {
			if n.IP == neighbor.IP && n.HostName == neighbor.HostName {
				c.Neighbor.Neighbors = slices.Delete(c.Neighbor.Neighbors, i, i+1)
			}
		}
	} else if neighbor.IP == "" && neighbor.HostName != "" {
		for i, n := range c.Neighbor.Neighbors {
			if n.HostName == neighbor.HostName {
				c.Neighbor.Neighbors = slices.Delete(c.Neighbor.Neighbors, i, i+1)
			}
		}
	} else if neighbor.IP != "" && neighbor.HostName == "" {
		for i, n := range c.Neighbor.Neighbors {
			if n.IP == neighbor.IP {
				c.Neighbor.Neighbors = slices.Delete(c.Neighbor.Neighbors, i, i+1)
			}
		}
	} else {
		for i, _ := range c.Neighbor.Neighbors {
			c.Neighbor.Neighbors = slices.Delete(c.Neighbor.Neighbors, i, i+1)
		}
	}
	SaveConfig()
	ctx.IndentedJSON(http.StatusOK, c.Neighbor)
}

// GetNeighborInfo godoc
//
//	@Summary		GetNeighbor
//	@Description	GetNeighbor.
//	@Tags			API, CUBE
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Success		200	{object}	TypeNeighborInfos
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/neighbor/info [get]
func (c *TypeController) GetNeighborInfo(ctx *gin.Context) {
	ctx.IndentedJSON(http.StatusOK, c.UpdateNeighborInfo())
}

func (n *TypeNeighbor) GetFromNeighbor(api string) (map[string]interface{}, int) {
	req, err := http.NewRequest("GET", "http://"+n.IP+":8080/api/"+api, nil)

	ret := map[string]interface{}{
		"err": nil,
	}
	if err != nil {
		fmt.Println(err)
	}
	req.Header.Add("User-Agent", "CubeAPI")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		controller.AddError(err)
		ret["err"] = err
		return ret, 404
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

			fmt.Println(err)
			controller.AddError(err)
			ret["err"] = err
			return
		}
	}(resp.Body)

	// 결과 출력

	bytes, _ := io.ReadAll(resp.Body)
	str := string(bytes) //바이트를 문자열로
	fmt.Println(str)
	err = json.Unmarshal([]byte(str), &ret)
	if err != nil {
		fmt.Println(err)
		controller.AddError(err)
		ret["err"] = err
		return ret, resp.StatusCode
	}
	return ret, resp.StatusCode
}

type TypeNeighborInfos struct {
	Neighbors map[string]TypeNeighborInfo `json:"neighbors"`
}
type TypeNeighborInfo struct {
	Info map[string]interface{} `json:"info"`
	Code int                    `json:"code"`
}

func (c *TypeController) UpdateNeighborInfo() TypeNeighborInfos {
	ret := TypeNeighborInfos{Neighbors: make(map[string]TypeNeighborInfo)}
	for _, neighbor := range c.Neighbor.Neighbors {
		str, code := neighbor.GetFromNeighbor("v1/cube/neighbor")
		ret.Neighbors[neighbor.HostName] = TypeNeighborInfo{str, code}
	}
	return ret
}
