package core

import (
	"fmt"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/json"
	"math"
	"os"
	"reflect"
	"runtime"
	"strings"
	"time"
)

// 顶层公共Config
type Config struct {
	MemTrace   MemTraceConfig
	PerfStat   PerfStatConfig
	Algorithm  AlgorithmConfig
	Kubernetes KubernetesConfig
	Manager    ManagerConfig
	Debug      DebugConfig
}

type RthCalculatorType string

var (
	RthCalculatorTypeReservoir RthCalculatorType = "reservoir"
	RthCalculatorTypeFull      RthCalculatorType = "full"
)

type MicroArchitectureName string

var (
	MicroArchitectureNameSkyLake     MicroArchitectureName = "SkyLake"
	MicroArchitectureNameCascadeLake MicroArchitectureName = "CascadeLake"
)

type MemTraceSampler string

var (
	MemTraceSamplerPerf MemTraceSampler = "perf"
	MemTraceSamplerPin  MemTraceSampler = "pin"
)

type MemTraceConfig struct {
	TraceCount        int
	MaxRthTime        int
	ConcurrentMax     int
	RthCalculatorType RthCalculatorType
	ReservoirSize     int
	Sampler           MemTraceSampler
	PinConfig         PinConfig
	PerfRecordConfig  PerfRecordConfig
}

type PerfRecordConfig struct {
	SwitchOutput  string
	OverflowCount int
	PerfExecPath  string
}

type PinConfig struct {
	PinPath        string
	PinToolPath    string
	BufferSize     int
	WriteThreshold int
}

type PerfStatConfig struct {
	MicroArchitecture MicroArchitectureName
	SampleTime        time.Duration
}

type ClassifyConfig struct {
	MPKIVeryHigh               float64
	MPKIHigh                   float64
	HPKIVeryHigh               float64
	HPKIVeryLow                float64
	IPCVeryLow                 float64
	IPCLow                     float64
	NonCriticalAPKI            float64
	NoChangeThreshold          float64
	SignificantChangeThreshold float64
	APKILow                    float64
}

type DCAPSConfig struct {
	MaxIteration                        int
	InitialStep                         float64
	MinStep                             float64
	StepReductionRatio                  float64
	InitialTemperature                  float64
	TemperatureMin                      float64
	TemperatureReductionRatio           float64
	K                                   float64 // 计算是否更改计划的概率公式常数。值越大，概率越大
	ProbabilityChangeScheme             float64
	AggregateChangeOfOccupancyThreshold int
}

type AlgorithmConfig struct {
	Classify ClassifyConfig
	DCAPS    DCAPSConfig
}

type KubernetesConfig struct {
	TokenFile string
	CAFile    string
	Insecure  bool
	Host      string
}

type ManagerConfig struct {
	AllocCoolDown               time.Duration // 再分配的冷却时间，避免频繁分配
	AllocSquash                 time.Duration // 在这个时间段内，多个分配请求合并到一次完成
	ChangeProcessCountThreshold int           // 多个进程组更新时，更新的进程的数量达到这个数字时才进行再分配
	TargetPrograms              []string      // 当使用ProcessWatcher时，监控的目标程序
	ClassifyAfter               time.Duration // 跳过应用启动的的初始化时间
}

type DebugConfig struct {
	IgnorePqosError bool // 即便PQOS设置失败，也不会返回错误。鉴于开发机没有CAT功能，打开此选项用于本地调试。
}

var RootConfig = &Config{
	MemTrace: MemTraceConfig{
		TraceCount:        1000000000,
		MaxRthTime:        100000,
		ConcurrentMax:     int(math.Min(math.Max(1, float64(runtime.NumCPU())/4), 4)),
		RthCalculatorType: RthCalculatorTypeReservoir,
		ReservoirSize:     100000,
		Sampler:           MemTraceSamplerPerf,
		PinConfig: PinConfig{
			PinPath:        "/home/wjx/bin/pin",
			PinToolPath:    "/home/wjx/Workspace/pin-3.17/source/tools/MemTrace2/obj-intel64/MemTrace2.so",
			BufferSize:     10000,
			WriteThreshold: 20000,
		},
		PerfRecordConfig: PerfRecordConfig{
			SwitchOutput:  "10M",
			OverflowCount: 5,
			PerfExecPath:  "/home/wjx/linux-5.4.0/tools/perf",
		},
	},
	PerfStat: PerfStatConfig{
		SampleTime:        30 * time.Second,
		MicroArchitecture: MicroArchitectureNameSkyLake,
	},
	Algorithm: AlgorithmConfig{
		Classify: ClassifyConfig{
			MPKIVeryHigh:               10,
			MPKIHigh:                   5,
			HPKIVeryHigh:               10,
			HPKIVeryLow:                0.5,
			IPCVeryLow:                 0.6,
			IPCLow:                     1.3,
			NonCriticalAPKI:            1,
			NoChangeThreshold:          0.1,
			SignificantChangeThreshold: 0.3,
			APKILow:                    1,
		},
		DCAPS: DCAPSConfig{
			MaxIteration:                        200,
			InitialStep:                         10000,
			MinStep:                             100,
			StepReductionRatio:                  0.8,
			InitialTemperature:                  10000,
			TemperatureMin:                      100,
			TemperatureReductionRatio:           0.8,
			K:                                   1,
			ProbabilityChangeScheme:             0.2,
			AggregateChangeOfOccupancyThreshold: 100,
		},
	},
	Manager: ManagerConfig{
		AllocCoolDown:               60 * time.Second,
		AllocSquash:                 50 * time.Millisecond,
		ChangeProcessCountThreshold: 100, // 暂定
		TargetPrograms: []string{"perlbench_r_base.shiyan-m64", "cpugcc_r_base.shiyan-m64", "mcf_r_base.shiyan-m64",
			"omnetpp_r_base.shiyan-m64", "x264_r_base.shiyan-m64", "deepsjeng_r_base.shiyan-m64", "cpuxalan_r_base.shiyan-m64",
			"leela_r_base.shiyan-m64", "exchange2_r_base.shiyan-m64", "xz_r_base.shiyan-m64"},
		ClassifyAfter: 5 * time.Second,
	},
	Debug: DebugConfig{
		IgnorePqosError: false,
	},
}

func checkNotZero(val reflect.Value, path []string) error {
	if val.Kind() == reflect.String {
		return nil
	} else if val.Kind() == reflect.Struct {
		typ := reflect.TypeOf(val)
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			err := checkNotZero(field, append(path, typ.Field(i).Name))
			if err != nil {
				return err
			}
		}
	} else if val.Kind() == reflect.Float64 {
		if val.Float() == 0 {
			return fmt.Errorf("字段 %s 为0", strings.Join(path, "."))
		}
	} else if val.Kind() == reflect.Int {
		if val.Int() == 0 {
			return fmt.Errorf("字段 %s 为0", strings.Join(path, "."))
		}
	} else {
		panic(fmt.Sprintf("没有遇到的类型 %s", val.Kind()))
	}
	return nil
}

func (config *Config) Check() error {
	pinToolPath := config.MemTrace.PinConfig.PinToolPath
	_, err := os.Stat(pinToolPath)
	if os.IsNotExist(err) {
		return errors.Wrap(err, fmt.Sprintf("无法访问PinTool路径%s", pinToolPath))
	}

	// 检查不能为0的字段
	err = checkNotZero(reflect.ValueOf(config), []string{})
	if err != nil {
		return err
	}

	return nil
}

func (config *Config) String() string {
	marshal, _ := json.Marshal(config)
	return string(marshal)
}
