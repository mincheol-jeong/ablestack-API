package glue

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/ycyun/Cube-API/internal/infra/utils"
	"os/exec"
	"reflect"
	"sync"
)

type TypeGlueClass struct {
	Type              string  `json:"type"`
	TotalBytes        int64   `json:"total_bytes"`
	TotalAvailBytes   int64   `json:"total_avail_bytes"`
	TotalUsedBytes    int64   `json:"total_used_bytes"`
	TotalUsedRawBytes int64   `json:"total_used_raw_bytes"`
	TotalUsedRawRatio float64 `json:"total_used_raw_ratio"`
}
type TypeGluePoolStats struct {
	Stored             int64   `json:"stored"`
	StoredData         int64   `json:"stored_data"`
	StoredOmap         int     `json:"stored_omap"`
	Objects            int     `json:"objects"`
	KbUsed             int64   `json:"kb_used"`
	BytesUsed          int64   `json:"bytes_used"`
	DataBytesUsed      int64   `json:"data_bytes_used"`
	OmapBytesUsed      int     `json:"omap_bytes_used"`
	PercentUsed        float64 `json:"percent_used"`
	MaxAvail           int64   `json:"max_avail"`
	QuotaObjects       int     `json:"quota_objects"`
	QuotaBytes         int     `json:"quota_bytes"`
	Dirty              int     `json:"dirty"`
	Rd                 int64   `json:"rd"`
	RdBytes            int64   `json:"rd_bytes"`
	Wr                 int64   `json:"wr"`
	WrBytes            int64   `json:"wr_bytes"`
	CompressBytesUsed  int64   `json:"compress_bytes_used"`
	CompressUnderBytes int64   `json:"compress_under_bytes"`
	StoredRaw          int64   `json:"stored_raw"`
	AvailRaw           int64   `json:"avail_raw"`
}

type TypeGluePools struct {
	Name  string            `json:"name"`
	Id    int               `json:"id"`
	Stats TypeGluePoolStats `json:"stats"`
}

type TypeGlueStorageStats struct {
	TotalBytes         int64   `json:"total_bytes"`
	TotalAvailBytes    int64   `json:"total_avail_bytes"`
	TotalUsedBytes     int64   `json:"total_used_bytes"`
	TotalUsedRawBytes  int64   `json:"total_used_raw_bytes"`
	TotalUsedRawRatio  float64 `json:"total_used_raw_ratio"`
	NumOsds            int     `json:"num_osds"`
	NumPerPoolOsds     int     `json:"num_per_pool_osds"`
	NumPerPoolOmapOsds int     `json:"num_per_pool_omap_osds"`
}

type TypeGlueStorageSize struct {
	Stats        TypeGlueStorageStats `json:"stats"`
	StatsByClass []TypeGlueClass      `json:"stats_by_class"`
	Pools        []TypeGluePools      `json:"pools"`
} // @name TypeGlueStorageSize

var lockGlueStorageSize sync.Once

var _glueStorageSize *TypeGlueStorageSize

func StorageSize() *TypeGlueStorageSize {
	if _glueStorageSize == nil {
		lockGlueStorageSize.Do(
			func() {
				if gin.IsDebugging() {
					fmt.Println("Creating ", reflect.TypeOf(_glueStorageSize), " now.")
				}
				_glueStorageSize = &TypeGlueStorageSize{}
			})
	} else {
		if gin.IsDebugging() {
			fmt.Println("get old ", reflect.TypeOf(_glueStorageSize), "instance.")
		}
	}
	return _glueStorageSize
}

func UpdateStorageSize() *TypeGlueStorageSize {
	StorageSize()
	var _tmpGlueStorageSize map[string]interface{}
	if gin.Mode() == gin.ReleaseMode {
		var stdout []byte

		cmd := exec.Command("ceph", "df", "detail", "-f", "json")
		stdout, _ = cmd.CombinedOutput()

		if err := json.Unmarshal(stdout, &_tmpGlueStorageSize); err != nil && gin.IsDebugging() {
			utils.FancyHandleError(err)

		}
	} else {

		stdout := []byte("{\n  \"stats\": {\n    \"total_bytes\": 18243678896128,\n    \"total_avail_bytes\": 13450613030912,\n    \"total_used_bytes\": 4793065865216,\n    \"total_used_raw_bytes\": 4793065865216,\n    \"total_used_raw_ratio\": 0.26272475719451904,\n    \"num_osds\": 19,\n    \"num_per_pool_osds\": 19,\n    \"num_per_pool_omap_osds\": 19\n  },\n  \"stats_by_class\": {\n    \"ssd\": {\n      \"total_bytes\": 18243678896128,\n      \"total_avail_bytes\": 13450613030912,\n      \"total_used_bytes\": 4793065865216,\n      \"total_used_raw_bytes\": 4793065865216,\n      \"total_used_raw_ratio\": 0.26272475719451904\n    }\n  },\n  \"pools\": [\n    {\n      \"name\": \".mgr\",\n      \"id\": 1,\n      \"stats\": {\n        \"stored\": 228852576,\n        \"stored_data\": 228852576,\n        \"stored_omap\": 0,\n        \"objects\": 55,\n        \"kb_used\": 446984,\n        \"bytes_used\": 457711616,\n        \"data_bytes_used\": 457711616,\n        \"omap_bytes_used\": 0,\n        \"percent_used\": 0.000037596404581563547,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 60143,\n        \"rd_bytes\": 174836736,\n        \"wr\": 124613,\n        \"wr_bytes\": 2982782976,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 457705152,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \"rbd\",\n      \"id\": 2,\n      \"stats\": {\n        \"stored\": 2767179923443,\n        \"stored_data\": 2767179743232,\n        \"stored_omap\": 180211,\n        \"objects\": 775249,\n        \"kb_used\": 4559950172,\n        \"bytes_used\": 4669388976102,\n        \"data_bytes_used\": 4669388615680,\n        \"omap_bytes_used\": 360422,\n        \"percent_used\": 0.27722570300102234,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 6298466507,\n        \"rd_bytes\": 89269259841536,\n        \"wr\": 2372197615,\n        \"wr_bytes\": 35909774067712,\n        \"compress_bytes_used\": 697354125312,\n        \"compress_under_bytes\": 1508618130432,\n        \"stored_raw\": 5534360010752,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \"cephfs.GlueFS.meta\",\n      \"id\": 3,\n      \"stats\": {\n        \"stored\": 267035157,\n        \"stored_data\": 266981552,\n        \"stored_omap\": 53605,\n        \"objects\": 89,\n        \"kb_used\": 521617,\n        \"bytes_used\": 534135499,\n        \"data_bytes_used\": 534028288,\n        \"omap_bytes_used\": 107211,\n        \"percent_used\": 0.000043873587856069207,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 1927,\n        \"rd_bytes\": 2682120192,\n        \"wr\": 2376944,\n        \"wr_bytes\": 5711469568,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 534070304,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \"cephfs.GlueFS.data\",\n      \"id\": 4,\n      \"stats\": {\n        \"stored\": 38484193280,\n        \"stored_data\": 38484193280,\n        \"stored_omap\": 0,\n        \"objects\": 9227,\n        \"kb_used\": 75164544,\n        \"bytes_used\": 76968493056,\n        \"data_bytes_used\": 76968493056,\n        \"omap_bytes_used\": 0,\n        \"percent_used\": 0.00628270348533988,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 15061768,\n        \"rd_bytes\": 31354227788800,\n        \"wr\": 19215663,\n        \"wr_bytes\": 1841239221248,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 76968386560,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \".rgw.root\",\n      \"id\": 5,\n      \"stats\": {\n        \"stored\": 1386,\n        \"stored_data\": 1386,\n        \"stored_omap\": 0,\n        \"objects\": 4,\n        \"kb_used\": 32,\n        \"bytes_used\": 32768,\n        \"data_bytes_used\": 32768,\n        \"omap_bytes_used\": 0,\n        \"percent_used\": 2.6916628925732766E-9,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 837,\n        \"rd_bytes\": 857088,\n        \"wr\": 0,\n        \"wr_bytes\": 0,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 2772,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \"default.rgw.log\",\n      \"id\": 6,\n      \"stats\": {\n        \"stored\": 3702,\n        \"stored_data\": 3702,\n        \"stored_omap\": 0,\n        \"objects\": 209,\n        \"kb_used\": 272,\n        \"bytes_used\": 278528,\n        \"data_bytes_used\": 278528,\n        \"omap_bytes_used\": 0,\n        \"percent_used\": 2.2879135030962061E-8,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 44388964,\n        \"rd_bytes\": 45540829184,\n        \"wr\": 29546709,\n        \"wr_bytes\": 25600,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 7404,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \"default.rgw.control\",\n      \"id\": 7,\n      \"stats\": {\n        \"stored\": 0,\n        \"stored_data\": 0,\n        \"stored_omap\": 0,\n        \"objects\": 8,\n        \"kb_used\": 0,\n        \"bytes_used\": 0,\n        \"data_bytes_used\": 0,\n        \"omap_bytes_used\": 0,\n        \"percent_used\": 0,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 0,\n        \"rd_bytes\": 0,\n        \"wr\": 0,\n        \"wr_bytes\": 0,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 0,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \"default.rgw.meta\",\n      \"id\": 8,\n      \"stats\": {\n        \"stored\": 4057,\n        \"stored_data\": 1870,\n        \"stored_omap\": 2187,\n        \"objects\": 11,\n        \"kb_used\": 69,\n        \"bytes_used\": 69910,\n        \"data_bytes_used\": 65536,\n        \"omap_bytes_used\": 4374,\n        \"percent_used\": 5.7426197130894252E-9,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 65172,\n        \"rd_bytes\": 54934528,\n        \"wr\": 439,\n        \"wr_bytes\": 130048,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 8114,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \"default.rgw.buckets.index\",\n      \"id\": 9,\n      \"stats\": {\n        \"stored\": 31629,\n        \"stored_data\": 0,\n        \"stored_omap\": 31629,\n        \"objects\": 11,\n        \"kb_used\": 62,\n        \"bytes_used\": 63259,\n        \"data_bytes_used\": 0,\n        \"omap_bytes_used\": 63259,\n        \"percent_used\": 5.1962860680987433E-9,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 100939,\n        \"rd_bytes\": 103372800,\n        \"wr\": 495,\n        \"wr_bytes\": 106496,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 63258,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \"default.rgw.buckets.data\",\n      \"id\": 11,\n      \"stats\": {\n        \"stored\": 1255955584,\n        \"stored_data\": 1255955584,\n        \"stored_omap\": 0,\n        \"objects\": 305,\n        \"kb_used\": 2453072,\n        \"bytes_used\": 2511945728,\n        \"data_bytes_used\": 2511945728,\n        \"omap_bytes_used\": 0,\n        \"percent_used\": 0.00020629627397283912,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 107,\n        \"rd_bytes\": 6218752,\n        \"wr\": 708,\n        \"wr_bytes\": 1475675136,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 2511911168,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \".nfs\",\n      \"id\": 12,\n      \"stats\": {\n        \"stored\": 0,\n        \"stored_data\": 0,\n        \"stored_omap\": 0,\n        \"objects\": 2,\n        \"kb_used\": 0,\n        \"bytes_used\": 0,\n        \"data_bytes_used\": 0,\n        \"omap_bytes_used\": 0,\n        \"percent_used\": 0,\n        \"max_avail\": 6086943309824,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 77,\n        \"rd_bytes\": 67584,\n        \"wr\": 42,\n        \"wr_bytes\": 34816,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 0,\n        \"avail_raw\": 12173886404033\n      }\n    },\n    {\n      \"name\": \"default.rgw.buckets.non-ec\",\n      \"id\": 13,\n      \"stats\": {\n        \"stored\": 16587,\n        \"stored_data\": 108,\n        \"stored_omap\": 16479,\n        \"objects\": 4,\n        \"kb_used\": 97,\n        \"bytes_used\": 98591,\n        \"data_bytes_used\": 49152,\n        \"omap_bytes_used\": 49439,\n        \"percent_used\": 8.0985635975139303E-9,\n        \"max_avail\": 4057962119168,\n        \"quota_objects\": 0,\n        \"quota_bytes\": 0,\n        \"dirty\": 0,\n        \"rd\": 109,\n        \"rd_bytes\": 77824,\n        \"wr\": 54,\n        \"wr_bytes\": 16384,\n        \"compress_bytes_used\": 0,\n        \"compress_under_bytes\": 0,\n        \"stored_raw\": 49761,\n        \"avail_raw\": 12173886404033\n      }\n    }\n  ]\n}")

		if err := json.Unmarshal(stdout, &_tmpGlueStorageSize); err != nil {
			utils.FancyHandleError(err)

		}
	}

	var StatsByClass []TypeGlueClass
	var Pools []TypeGluePools
	for key, value := range _tmpGlueStorageSize {
		if key == "stats" {
			s := TypeGlueStorageStats{}
			marshal, err := json.MarshalIndent(value, "", "\t")
			if err != nil {
				return nil
			}
			err = json.Unmarshal(marshal, &s)
			if err != nil {
				return nil
			}
			_glueStorageSize.Stats = s
		} else if key == "stats_by_class" {
			for name, item := range value.(map[string]interface{}) {

				s := TypeGlueClass{}
				marshal, err := json.MarshalIndent(item, "", "\t")
				if err != nil {
					return nil
				}
				err = json.Unmarshal(marshal, &s)
				if err != nil {
					return nil
				}
				s.Type = name
				StatsByClass = append(StatsByClass, s)
			}
		} else if key == "pools" {
			for _, item := range value.([]interface{}) {

				s := TypeGluePools{}
				marshal, err := json.MarshalIndent(item, "", "\t")
				if err != nil {
					return nil
				}
				err = json.Unmarshal(marshal, &s)
				if err != nil {
					return nil
				}
				Pools = append(Pools, s)
			}
		}

	}

	_glueStorageSize.Pools = Pools
	_glueStorageSize.StatsByClass = StatsByClass
	return _glueStorageSize
}
