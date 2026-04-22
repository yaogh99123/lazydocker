#!/bin/bash

# 定义颜色
COLOR_BLUE='\033[0;34m'
COLOR_GREEN='\033[0;32m'
COLOR_RED='\033[0;31m'
COLOR_YELLOW='\033[0;33m'
COLOR_NC='\033[0m'

# 定义目标二进制名称
TARGET="lazydocker"

echo -e "${COLOR_BLUE}========================================${COLOR_NC}"
echo -e "${COLOR_BLUE}      DevOS 构建脚本 (Lazydocker)${COLOR_NC}"
echo -e "${COLOR_BLUE}========================================${COLOR_NC}"

# 1. 检查 Go 环境
if ! command -v go &> /dev/null; then
    echo -e "${COLOR_RED}错误: 未检测到 Go 环境，请先安装 Go。${COLOR_NC}"
    exit 1
fi

# 2. 清理旧文件
echo -e "${COLOR_YELLOW}正在清理旧构建...${COLOR_NC}"
rm -f "$TARGET"
rm -f "main"

# 3. 运行构建
# -ldflags "-s -w" 可以减小二进制体积
echo -e "${COLOR_BLUE}正在执行 go build -o $TARGET main.go ...${COLOR_NC}"
go build -ldflags "-s -w" -o "$TARGET" main.go

# 4. 验证结果
if [ $? -eq 0 ]; then
    echo -e "${COLOR_GREEN}----------------------------------------${COLOR_NC}"
    echo -e "${COLOR_GREEN}构建成功!${COLOR_NC}"
    echo -e "${COLOR_GREEN}文件位置: $(pwd)/$TARGET${COLOR_NC}"
    echo -e "${COLOR_GREEN}文件大小: $(ls -lh $TARGET | awk '{print $5}')${COLOR_NC}"
    echo -e "${COLOR_GREEN}----------------------------------------${COLOR_NC}"
    echo -e "运行示例:"
    echo -e "${COLOR_BLUE}  ./$TARGET -f <docker-compose-path> --devos${COLOR_NC}"
else
    echo -e "${COLOR_RED}构建失败，请检查代码错误。${COLOR_NC}"
    exit 1
fi
