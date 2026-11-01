// MIT License
//
// Copyright (c) 2025-2026 Arsene Tochemey Gandote
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package log

import (
	"fmt"
	"io"
	golog "log"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// DefaultLogger represents the default Log to use
// This Log wraps zap under the hood
var DefaultLogger = New(DebugLevel, os.Stdout)
var DiscardLogger = New(DebugLevel, io.Discard)

// Log implements Logger interface with the underlying zap as
// the underlying logging library
type Log struct {
	logger  *zap.Logger
	outputs []io.Writer
}

// enforce compilation and linter error
var _ Logger = &Log{}

// encoderConfig is the zap encoder configuration shared by every Log instance.
// It is copied from "zap.NewProductionEncoderConfig" with some updates.
var encoderConfig = zapcore.EncoderConfig{
	TimeKey:       "ts",
	LevelKey:      "level",
	NameKey:       "logger",
	CallerKey:     "caller",
	MessageKey:    "msg",
	StacktraceKey: "stacktrace",
	LineEnding:    zapcore.DefaultLineEnding,
	EncodeLevel:   zapcore.LowercaseLevelEncoder,

	// Custom EncodeTime function to ensure we match format and precision of historic capnslog timestamps
	EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02T15:04:05.000000Z0700"))
	},

	EncodeDuration: zapcore.StringDurationEncoder,
	EncodeCaller:   zapcore.ShortCallerEncoder,
}

// zapLevels maps the Log levels to their zap counterparts.
var zapLevels = map[Level]zapcore.Level{
	InfoLevel:    zapcore.InfoLevel,
	DebugLevel:   zapcore.DebugLevel,
	WarningLevel: zapcore.WarnLevel,
	ErrorLevel:   zapcore.ErrorLevel,
	PanicLevel:   zapcore.PanicLevel,
	FatalLevel:   zapcore.FatalLevel,
}

// New creates an instance of Log.
//
// The returned logger is self-contained: it does not touch zap's global
// logger nor the standard library's log package.
func New(level Level, writers ...io.Writer) *Log {
	// create the list of writers
	syncWriters := make([]zapcore.WriteSyncer, len(writers))
	for i, writer := range writers {
		syncWriters[i] = zapcore.AddSync(writer)
	}

	// set the log level; unknown levels default to debug
	zapLevel, ok := zapLevels[level]
	if !ok {
		zapLevel = zapcore.DebugLevel
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zap.CombineWriteSyncers(syncWriters...),
		zapLevel,
	)

	// get the zap Log
	zapLogger := zap.New(core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.PanicLevel),
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.AddStacktrace(zapcore.FatalLevel))

	// create the instance of Log and returns it
	return &Log{
		logger:  zapLogger,
		outputs: writers,
	}
}

// Debug starts a message with debug level
func (l *Log) Debug(v ...any) {
	l.logger.Sugar().Debug(fmt.Sprint(v...))
}

// Debugf starts a message with debug level
func (l *Log) Debugf(format string, v ...any) {
	l.logger.Sugar().Debug(fmt.Sprintf(format, v...))
}

// Panic starts a new message with panic level. The panic() function
// is called which stops the ordinary flow of a goroutine.
func (l *Log) Panic(v ...any) {
	l.logger.Sugar().Panic(fmt.Sprint(v...))
}

// Panicf starts a new message with panic level. The panic() function
// is called which stops the ordinary flow of a goroutine.
func (l *Log) Panicf(format string, v ...any) {
	l.logger.Sugar().Panic(fmt.Sprintf(format, v...))
}

// Fatal starts a new message with fatal level. The os.Exit(1) function
// is called which terminates the program immediately.
func (l *Log) Fatal(v ...any) {
	l.logger.Sugar().Fatal(fmt.Sprint(v...))
}

// Fatalf starts a new message with fatal level. The os.Exit(1) function
// is called which terminates the program immediately.
func (l *Log) Fatalf(format string, v ...any) {
	l.logger.Sugar().Fatal(fmt.Sprintf(format, v...))
}

// Error starts a new message with error level.
func (l *Log) Error(v ...any) {
	l.logger.Sugar().Error(fmt.Sprint(v...))
}

// Errorf starts a new message with error level.
func (l *Log) Errorf(format string, v ...any) {
	l.logger.Sugar().Error(fmt.Sprintf(format, v...))
}

// Warn starts a new message with warn level
func (l *Log) Warn(v ...any) {
	l.logger.Sugar().Warn(fmt.Sprint(v...))
}

// Warnf starts a new message with warn level
func (l *Log) Warnf(format string, v ...any) {
	l.logger.Sugar().Warn(fmt.Sprintf(format, v...))
}

// Info starts a message with info level
func (l *Log) Info(v ...any) {
	l.logger.Sugar().Info(fmt.Sprint(v...))
}

// Infof starts a message with info level
func (l *Log) Infof(format string, v ...any) {
	l.logger.Sugar().Info(fmt.Sprintf(format, v...))
}

// LogLevel returns the log level that is used
func (l *Log) LogLevel() Level {
	switch l.logger.Level() {
	case zapcore.FatalLevel:
		return FatalLevel
	case zapcore.PanicLevel:
		return PanicLevel
	case zapcore.ErrorLevel:
		return ErrorLevel
	case zapcore.InfoLevel:
		return InfoLevel
	case zapcore.DebugLevel:
		return DebugLevel
	case zapcore.WarnLevel:
		return WarningLevel
	default:
		return InvalidLevel
	}
}

// LogOutput returns the log output that is set
func (l *Log) LogOutput() []io.Writer {
	return l.outputs
}

// StdLogger returns a standard library logger that writes through this
// logger at its configured level. It does not redirect the global standard
// library logger.
func (l *Log) StdLogger() *golog.Logger {
	stdlogger, err := zap.NewStdLogAt(l.logger, l.logger.Level())
	if err != nil {
		// fall back to a plain standard logger writing to the same outputs
		return golog.New(io.MultiWriter(l.outputs...), "", golog.LstdFlags)
	}
	return stdlogger
}
