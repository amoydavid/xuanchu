#!/usr/bin/env node

/**
 * xuanchu MCP stdio 启动器
 * 
 * 用法：
 *   node wrapper.js
 *   node wrapper.js --db /path/to/xuanchu.db
 */

const { spawn } = require('child_process');
const path = require('path');

// 获取 xuanchu 二进制文件路径
const xuanchuPath = path.join(__dirname, 'xuanchu');

// 构建参数
const args = ['mcp', 'stdio'];

// 解析额外的参数（如 --db）
const userArgs = process.argv.slice(2);
if (userArgs.length > 0) {
  args.unshift(...userArgs);
}

// 启动 xuanchu 进程
const child = spawn(xuanchuPath, args, {
  stdio: ['pipe', 'pipe', 'pipe'],
  cwd: __dirname
});

// 将子进程的 stdio 与当前进程连接
process.stdin.pipe(child.stdin);
child.stdout.pipe(process.stdout);
child.stderr.pipe(process.stderr);

// 处理进程退出
child.on('exit', (code, signal) => {
  if (code !== null) {
    process.exit(code);
  } else if (signal) {
    process.kill(process.pid, signal);
  }
});

// 处理错误
child.on('error', (err) => {
  console.error('启动 xuanchu 失败:', err.message);
  process.exit(1);
});

// 处理当前进程信号
process.on('SIGINT', () => {
  child.kill('SIGINT');
});

process.on('SIGTERM', () => {
  child.kill('SIGTERM');
});

// 处理管道断开
process.stdin.on('end', () => {
  child.stdin.end();
});
