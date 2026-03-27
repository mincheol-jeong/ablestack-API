package glue

//
//type gluePools struct {
//	Pools []string `json:"pools"`
//} // @name GluePools
//
//type glueError struct {
//	utils.HTTPError
//	Detail    string `json:"detail"`
//	Code      string `json:"code"`
//	Component string `json:"component"`
//} // @name GlueError

type glueFS struct {
	Name string `json:"name"`
} // @name GlueFS

type FSList struct {
	GlueFS []glueFS `json:"glueFS"`
} // @name FSList

type GlueFSInfo struct {
	MonAddrs                  []string `json:"mon_addrs"`
	PendingSubvolumeDeletions int      `json:"pending_subvolume_deletions"`
	Pools                     struct {
		Data []struct {
			Avail int64  `json:"avail"`
			Name  string `json:"name"`
			Used  int    `json:"used"`
		} `json:"data"`
		Metadata []struct {
			Avail int64  `json:"avail"`
			Name  string `json:"name"`
			Used  int    `json:"used"`
		} `json:"metadata"`
	} `json:"pools"`
	UsedSize int `json:"used_size"`
} // @name GlueFSInfo

type User struct {
	Username string `json:"username" form:"username" uri:"username"`
} // @name user
