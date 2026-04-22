package app

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/jesseduffield/lazydocker/pkg/commands"
)

// 颜色定义
const (
	ColorRed    = "\033[0;31m"
	ColorGreen  = "\033[0;32m"
	ColorYellow = "\033[1;33m"
	ColorBlue   = "\033[0;34m"
	ColorNC     = "\033[0m" // No Color
)

// RunDevOSMode runs the application in DevOS interactive mode
func (app *App) RunDevOSMode() error {
	reader := bufio.NewReader(os.Stdin)

	for {
		// 1. Refresh data
		_, services, err := app.DockerCommand.RefreshContainersAndServices(nil)
		if err != nil {
			return err
		}

		// 按服务名称排序，确保序列稳定
		sort.Slice(services, func(i, j int) bool {
			return services[i].Name < services[j].Name
		})

		// 2. Clear screen
		fmt.Print("\033[H\033[2J")

		// 3. Render Header (Matching Dpanel.sh style)
		fmt.Printf("%s========================================%s\n", ColorBlue, ColorNC)
		fmt.Printf("%s      DevOS 服务管理%s\n", ColorBlue, ColorNC)
		fmt.Printf("%s========================================%s\n", ColorBlue, ColorNC)
		if len(app.DockerCommand.Config.ComposeFiles) > 0 {
			fmt.Printf("%s加载的文件: %s%s\n", ColorYellow, strings.Join(app.DockerCommand.Config.ComposeFiles, ", "), ColorNC)
		} else {
			fmt.Printf("%s加载的文件: 默认 (docker-compose.yml)%s\n", ColorYellow, ColorNC)
		}
		fmt.Println("")
		fmt.Printf("%s=== 所有服务列表 ===%s\n\n", ColorBlue, ColorNC)

		// 4. Render Services
		for i, svc := range services {
			status := fmt.Sprintf("%s未运行%s", ColorRed, ColorNC)
			if svc.Container != nil {
				state := svc.Container.Container.State
				if state == "running" {
					status = fmt.Sprintf("%s运行中%s", ColorGreen, ColorNC)
				} else if state == "exited" {
					status = fmt.Sprintf("%s已停止%s", ColorYellow, ColorNC)
				} else {
					status = fmt.Sprintf("%s%s%s", ColorYellow, state, ColorNC)
				}
			}

			desc := ""
			if svc.Description != "" {
				desc = fmt.Sprintf("%s# %s%s", ColorYellow, svc.Description, ColorNC)
			}
			fmt.Printf("%2d. %-20s [%s] %s\n", i+1, svc.Name, status, desc)
		}

		// 5. Render Menu (Matching Dpanel.sh)
		fmt.Printf("\n%s功能菜单:%s\n", ColorGreen, ColorNC)
		fmt.Println("  1. 启动服务 (所有/指定)")
		fmt.Println("  2. 停止服务 (所有/指定)")
		fmt.Println("  3. 重启服务 (所有/指定)")
		fmt.Println("  4. 查看日志 (所有/指定)")
		fmt.Println("  5. 查看服务状态 (指定)")
		fmt.Println("  6. 查看服务配置 (指定)")
		fmt.Println("  7. 进入容器 (指定)")
		fmt.Println("  8. 编译服务 (所有/指定)")
		fmt.Println("  9. 清理服务 (所有/指定)")
		fmt.Println(" 10. 删除镜像 (所有/指定)")
		fmt.Println(" 11. 一键启动日志监控服务栈 (elasticsearch,filebeat,go-stash,grafana,jaeger,kafka,zookeeper)")
		fmt.Println(" 12. 一键启动数据库服务栈 (clickhouse,mysql,redis)")
		fmt.Println(" 13. 清理 Docker build 缓存")
		fmt.Println(" 14. 清理 Docker buildx 缓存")
		fmt.Println(" 15. 网络管理")
		fmt.Println(" 16. 卷管理")
		fmt.Println("100. 修复服务 (所有/指定) - 重新构建镜像")
		fmt.Println("  0. 退出")

		fmt.Printf("\n请选择功能 [0-16,100]: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "0" {
			fmt.Printf("%s再见!%s\n", ColorGreen, ColorNC)
			break
		}

		app.handleDevOSInput(input, services, reader)
	}

	return nil
}

func (app *App) handleDevOSInput(choice string, services []*commands.Service, reader *bufio.Reader) {
	switch choice {
	case "1":
		app.doServiceAction("启动", services, reader, true, true, func(s *commands.Service) error {
			return s.Up()
		})
	case "2":
		app.doServiceAction("停止", services, reader, true, true, func(s *commands.Service) error {
			return s.Stop()
		})
	case "3":
		app.doServiceAction("重启", services, reader, true, true, func(s *commands.Service) error {
			return s.Restart()
		})
	case "4":
		app.doServiceAction("查看日志", services, reader, true, false, func(s *commands.Service) error {
			if s.Container == nil {
				fmt.Printf("%s警告: 服务 %s 未运行，可能无实时日志。%s\n", ColorYellow, s.Name, ColorNC)
			}
			fmt.Printf("\n%s--- 正在查看服务日志: %s (输入 'exit' 返回主菜单) ---%s\n", ColorBlue, s.Name, ColorNC)
			cmd, err := s.ViewLogs()
			if err != nil {
				return err
			}
			return app.runSubprocessWithQuitKey(cmd)
		})
	case "5":
		app.doServiceAction("查看状态", services, reader, false, false, func(s *commands.Service) error {
			fmt.Printf("%s=== 服务状态: %s ===%s\n", ColorBlue, s.Name, ColorNC)
			composeCommand := app.getComposeCommandWithFiles()
			fullCmd := fmt.Sprintf("%s ps %s", composeCommand, s.Name)
			cmd := exec.Command("sh", "-c", fullCmd)
			return app.runSubprocessWithQuitKey(cmd)
		})
	case "6":
		app.doServiceAction("查看配置", services, reader, false, false, func(s *commands.Service) error {
			fmt.Printf("%s=== 服务配置: %s ===%s\n", ColorBlue, s.Name, ColorNC)
			composeCommand := app.getComposeCommandWithFiles()
			fullCmd := fmt.Sprintf("%s config %s", composeCommand, s.Name)
			cmd := exec.Command("sh", "-c", fullCmd)
			return app.runSubprocessWithQuitKey(cmd)
		})
	case "7":
		app.doServiceAction("进入容器", services, reader, false, false, func(s *commands.Service) error {
			if s.Container == nil {
				return fmt.Errorf("服务 %s 未运行", s.Name)
			}
			fmt.Printf("%s--- 正在进入容器: %s (输入 'exit' 退出) ---%s\n", ColorBlue, s.Name, ColorNC)
			
			// 1. 先静默探测是否存在 bash
			checkCmd := exec.Command("docker", "exec", s.Container.ID, "which", "bash")
			if err := checkCmd.Run(); err == nil {
				// 2. 探测成功，进入 bash
				cmd := exec.Command("docker", "exec", "-it", s.Container.ID, "bash")
				return app.runInteractiveSubprocess(cmd)
			}

			// 3. 探测失败，进入 sh
			cmd := exec.Command("docker", "exec", "-it", s.Container.ID, "sh")
			return app.runInteractiveSubprocess(cmd)
		})
	case "8":
		app.doServiceAction("编译(更新服务)", services, reader, true, false, func(s *commands.Service) error {
			composeCommand := app.getComposeCommandWithFiles()
			// 先 pull 确保镜像最新，再 build。通过 grep 过滤掉没必要的警告信息。
			fullCmd := fmt.Sprintf("%s pull %s && %s build --no-cache %s 2>&1 | grep -v 'No services to build' || true", 
				composeCommand, s.Name, composeCommand, s.Name)
			cmd := exec.Command("sh", "-c", fullCmd)
			return app.runSubprocessWithQuitKey(cmd)
		})
	case "9":
		app.doServiceAction("清理", services, reader, true, true, func(s *commands.Service) error {
			fmt.Printf("%s警告: 这将停止并删除服务: %s%s\n", ColorYellow, s.Name, ColorNC)
			fmt.Printf("确定要继续吗? (y/n): ")
			confirm, _ := reader.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
				return nil
			}
			if s.Container == nil {
				return nil
			}
			if err := s.Stop(); err != nil {
				return err
			}
			return s.Remove(container.RemoveOptions{Force: true})
		})
	case "10":
		app.doServiceAction("删除镜像", services, reader, true, false, func(s *commands.Service) error {
			if s.Container == nil {
				return fmt.Errorf("无法获取服务 %s 的快照信息", s.Name)
			}
			imageID := s.Container.Container.ImageID
			fmt.Printf("%s正在删除服务 %s 的镜像 (ID: %s)...%s\n", ColorYellow, s.Name, imageID, ColorNC)
			cmd := exec.Command("docker", "rmi", "-f", imageID)
			return app.runSubprocessWithQuitKey(cmd)
		})
	case "11":
		stack := []string{"zookeeper", "kafka", "elasticsearch", "filebeat", "go-stash", "jaeger", "grafana"}
		app.runStackAction("日志监控服务栈", stack, services, reader)
	case "12":
		stack := []string{"clickhouse", "mysql", "redis"}
		app.runStackAction("数据库服务栈", stack, services, reader)
	case "13":
		fmt.Printf("\n%s正在清理 Docker 构建缓存...%s\n", ColorBlue, ColorNC)
		cmd := exec.Command("docker", "builder", "prune", "-f")
		app.runSubprocessWithQuitKey(cmd)
	case "14":
		fmt.Printf("\n%s正在清理 Docker 构建历史(含 buildx)...%s\n", ColorBlue, ColorNC)
		// buildx 通常使用 builder prune 即可，-a 清理所有，--force 强制执行
		cmd := exec.Command("docker", "builder", "prune", "-af")
		app.runSubprocessWithQuitKey(cmd)
	case "15":
		app.runNetworkManagement(reader)
	case "16":
		app.runVolumeManagement(reader)
	case "100":
		app.doServiceAction("修复", services, reader, true, false, func(s *commands.Service) error {
			fmt.Printf("%s正在全面修复服务: %s...%s\n", ColorYellow, s.Name, ColorNC)
			if s.Container != nil {
				s.Stop()
				s.Remove(container.RemoveOptions{Force: true})
				exec.Command("docker", "rmi", "-f", s.Container.Container.ImageID).Run()
			}
			composeCommand := app.getComposeCommandWithFiles()
			buildCmd := fmt.Sprintf("%s build --no-cache %s", composeCommand, s.Name)
			exec.Command("sh", "-c", buildCmd).Run()
			return s.Up()
		})
	}
}

func (app *App) doServiceAction(actionName string, services []*commands.Service, reader *bufio.Reader, allowAll bool, waitForEnter bool, action func(*commands.Service) error) {
	fmt.Printf("\n%s选择要%s的服务（直接按回车或输入 q/0 返回主菜单）：%s\n", ColorYellow, actionName, ColorNC)
	if allowAll {
		fmt.Println("输入 'all' 选择所有服务，或输入数字索引 (如: 1) 或服务名 (如: mysql)，多个用空格分隔")
	} else {
		fmt.Println("输入数字索引 (如: 1) 或服务名 (如: mysql)，多个用空格分隔")
	}
	fmt.Print("服务名/索引：")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" || input == "q" || input == "0" {
		return
	}

	var targets []*commands.Service
	if allowAll && input == "all" {
		targets = services
	} else {
		parts := strings.Fields(input)
		for _, p := range parts {
			var idx int
			_, err := fmt.Sscanf(p, "%d", &idx)
			if err == nil && idx > 0 && idx <= len(services) {
				targets = append(targets, services[idx-1])
			} else {
				for _, s := range services {
					if s.Name == p {
						targets = append(targets, s)
						break
					}
				}
			}
		}
	}

	if len(targets) == 0 {
		fmt.Printf("%s错误: 未找到匹配的服务%s\n", ColorRed, ColorNC)
		return
	}

	for _, s := range targets {
		fmt.Printf("\n%s正在执行 %s: %s...%s\n", ColorBlue, actionName, s.Name, ColorNC)
		if err := action(s); err != nil {
			fmt.Printf("%s失败: %v%s\n", ColorRed, err, ColorNC)
		} else {
			fmt.Printf("%s成功%s\n", ColorGreen, ColorNC)
		}
	}

	if waitForEnter {
		fmt.Printf("\n%s按 Enter 继续...%s", ColorYellow, ColorNC)
		reader.ReadString('\n')
	}
}

func (app *App) runStackAction(stackName string, stack []string, services []*commands.Service, reader *bufio.Reader) {
	fmt.Printf("\n%s确定要一键启动%s吗? (y/n, 默认为 n): %s", ColorBlue, stackName, ColorNC)
	confirm, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
		return
	}

	fmt.Printf("\n%s正在启动%s...%s\n", ColorBlue, stackName, ColorNC)
	fmt.Printf("%s包含服务: %s%s\n", ColorYellow, strings.Join(stack, ", "), ColorNC)

	var targets []*commands.Service
	for _, name := range stack {
		for _, s := range services {
			if s.Name == name {
				targets = append(targets, s)
				break
			}
		}
	}

	if len(targets) == 0 {
		fmt.Printf("%s错误: 栈内服务均未在当前配置中找到%s\n", ColorRed, ColorNC)
		return
	}

	for _, s := range targets {
		fmt.Printf("启动 %s...\n", s.Name)
		s.Up()
	}
}

func (app *App) runSubprocessWithQuitKey(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 为子进程创建一个单独的管道，用于在特殊情况下强制杀掉它
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// 捕获系统中断，防止主程序退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// 监听 'exit' 命令来退出
	quitChan := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text == "exit" {
				quitChan <- true
				return
			}
			// 如果只是按了回车或其他，不做任何操作，继续等待
		}
	}()

	select {
	case err := <-done:
		if err != nil {
			fmt.Printf("\n%s指令执行出错: %v%s\n", ColorRed, err, ColorNC)
		}
		fmt.Printf("\n%s--- 执行完毕，输入 'exit' 返回主菜单 ---%s\n", ColorBlue, ColorNC)
		// 子进程虽然结束了，但我们要继续等待 quitChan 里的 'exit' 命令
		goto WAIT_LOOP
	case <-sigChan:
		// 收到 Ctrl+C，打印提示但不退出
		fmt.Printf("\n%s[提示] 请输入 'exit' 并回车以返回主菜单%s\n", ColorYellow, ColorNC)
		goto WAIT_LOOP
	case <-quitChan:
		// 收到 exit 命令，如果子进程还在跑，就杀掉它
		cmd.Process.Signal(os.Interrupt)
		fmt.Printf("\n%s正在返回主菜单...%s\n", ColorBlue, ColorNC)
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			cmd.Process.Kill()
		}
		return nil
	}

WAIT_LOOP:
	// 无论是因为子进程结束还是收到信号，都必须等到 quitChan 收到 'exit' 为止
	for {
		select {
		case <-sigChan:
			fmt.Printf("\n%s[提示] 必须输入 'exit' 才能退出当前界面%s\n", ColorYellow, ColorNC)
		case <-quitChan:
			return nil
		case <-done:
			// 这种情况下进程已经通过 done 退出了，不需要再处理，只需处理 quitChan
		}
	}
}

// getComposeCommandWithFiles 返回基础的 compose 命令
// 注意：NewAppConfig 已经将 -f 参数合并到了 UserConfig.CommandTemplates.DockerCompose 中
func (app *App) getComposeCommandWithFiles() string {
	return app.DockerCommand.Config.UserConfig.CommandTemplates.DockerCompose
}

// runInteractiveSubprocess 专门用于需要完全控制 Stdin 的场景（如 docker exec -it）
func (app *App) runInteractiveSubprocess(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// 捕获信号，但不做特殊处理，让它透传给子进程
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-sigChan:
		// 收到信号时，将信号发给子进程
		cmd.Process.Signal(os.Interrupt)
		// 等待子进程退出
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			cmd.Process.Kill()
		}
		return nil
	}
}

func (app *App) runNetworkManagement(reader *bufio.Reader) {
	for {
		networks, err := app.DockerCommand.RefreshNetworks()
		if err != nil {
			fmt.Printf("%s错误: %v%s\n", ColorRed, err, ColorNC)
			return
		}

		fmt.Printf("\n%s=== 网络管理 ===%s\n\n", ColorBlue, ColorNC)
		for i, nw := range networks {
			fmt.Printf("%2d. %-30s ID: %s\n", i+1, nw.Name, nw.Network.ID[:12])
		}

		fmt.Printf("\n%s功能:%s\n", ColorGreen, ColorNC)
		fmt.Println("  1. 创建网络")
		fmt.Println("  2. 删除网络")
		fmt.Println("  3. 容器加入网络")
		fmt.Println("  4. 容器退出网络")
		fmt.Println("  5. 清理未使用的网络 (Prune)")
		fmt.Println("  0. 返回主菜单")
		fmt.Print("\n请选择 [0-5]: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "0" || input == "" {
			break
		}

		switch input {
		case "1":
			fmt.Print("请输入网络名称: ")
			name, _ := reader.ReadString('\n')
			name = strings.TrimSpace(name)
			if name != "" {
				fmt.Printf("%s正在创建网络 %s...%s\n", ColorYellow, name, ColorNC)
				cmd := exec.Command("docker", "network", "create", name)
				if err := cmd.Run(); err != nil {
					fmt.Printf("%s失败: %v%s\n", ColorRed, err, ColorNC)
				} else {
					fmt.Printf("%s成功%s\n", ColorGreen, ColorNC)
				}
			}
		case "2":
			fmt.Print("请输入要删除的网络索引: ")
			idxStr, _ := reader.ReadString('\n')
			var idx int
			fmt.Sscanf(strings.TrimSpace(idxStr), "%d", &idx)
			if idx > 0 && idx <= len(networks) {
				nw := networks[idx-1]
				fmt.Printf("%s正在删除网络 %s...%s\n", ColorYellow, nw.Name, ColorNC)
				if err := nw.Remove(); err != nil {
					fmt.Printf("%s失败: %v%s\n", ColorRed, err, ColorNC)
				} else {
					fmt.Printf("%s成功%s\n", ColorGreen, ColorNC)
				}
			} else {
				fmt.Printf("%s无效的索引%s\n", ColorRed, ColorNC)
			}
		case "3":
			app.handleNetworkConnection(reader, networks, true)
		case "4":
			app.handleNetworkConnection(reader, networks, false)
		case "5":
			fmt.Printf("%s正在清理未使用的网络...%s\n", ColorYellow, ColorNC)
			if err := app.DockerCommand.PruneNetworks(); err != nil {
				fmt.Printf("%s失败: %v%s\n", ColorRed, err, ColorNC)
			} else {
				fmt.Printf("%s成功%s\n", ColorGreen, ColorNC)
			}
		}
	}
}

func (app *App) handleNetworkConnection(reader *bufio.Reader, networks []*commands.Network, isConnect bool) {
	action := "加入"
	cmdPart := "connect"
	if !isConnect {
		action = "退出"
		cmdPart = "disconnect"
	}

	// 1. 选择网络
	fmt.Printf("\n选择要%s的网络索引: ", action)
	idxStr, _ := reader.ReadString('\n')
	var netIdx int
	fmt.Sscanf(strings.TrimSpace(idxStr), "%d", &netIdx)
	if netIdx <= 0 || netIdx > len(networks) {
		fmt.Printf("%s无效的索引%s\n", ColorRed, ColorNC)
		return
	}
	targetNet := networks[netIdx-1]

	// 2. 选择容器
	containers, _, err := app.DockerCommand.RefreshContainersAndServices(nil)
	if err != nil {
		fmt.Printf("%s无法获取容器列表: %v%s\n", ColorRed, err, ColorNC)
		return
	}

	fmt.Printf("\n--- 容器列表 ---\n")
	for i, c := range containers {
		fmt.Printf("%2d. %-30s ID: %s\n", i+1, c.Name, c.ID[:12])
	}
	fmt.Printf("\n选择要%s网络的容器索引: ", action)
	cIdxStr, _ := reader.ReadString('\n')
	var cIdx int
	fmt.Sscanf(strings.TrimSpace(cIdxStr), "%d", &cIdx)
	if cIdx <= 0 || cIdx > len(containers) {
		fmt.Printf("%s无效的索引%s\n", ColorRed, ColorNC)
		return
	}
	targetContainer := containers[cIdx-1]

	// 3. 执行操作
	fmt.Printf("%s正在执行容器 %s %s网络 %s...%s\n", ColorYellow, targetContainer.Name, action, targetNet.Name, ColorNC)
	cmd := exec.Command("docker", "network", cmdPart, targetNet.Name, targetContainer.ID)
	if err := cmd.Run(); err != nil {
		fmt.Printf("%s失败: %v%s\n", ColorRed, err, ColorNC)
	} else {
		fmt.Printf("%s成功%s\n", ColorGreen, ColorNC)
	}
}

func (app *App) runVolumeManagement(reader *bufio.Reader) {
	for {
		volumes, err := app.DockerCommand.RefreshVolumes()
		if err != nil {
			fmt.Printf("%s错误: %v%s\n", ColorRed, err, ColorNC)
			return
		}

		fmt.Printf("\n%s=== 卷管理 ===%s\n\n", ColorBlue, ColorNC)
		for i, vol := range volumes {
			fmt.Printf("%2d. %-30s Driver: %s\n", i+1, vol.Name, vol.Volume.Driver)
		}

		fmt.Printf("\n%s功能:%s\n", ColorGreen, ColorNC)
		fmt.Println("  1. 清理未使用的卷 (Prune)")
		fmt.Println("  2. 删除指定卷 (按索引)")
		fmt.Println("  0. 返回主菜单")
		fmt.Print("\n请选择: ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "0" || input == "" {
			break
		}

		switch input {
		case "1":
			fmt.Printf("%s正在清理未使用的卷...%s\n", ColorYellow, ColorNC)
			if err := app.DockerCommand.PruneVolumes(); err != nil {
				fmt.Printf("%s失败: %v%s\n", ColorRed, err, ColorNC)
			} else {
				fmt.Printf("%s成功%s\n", ColorGreen, ColorNC)
			}
		case "2":
			fmt.Print("请输入要删除的卷索引: ")
			idxStr, _ := reader.ReadString('\n')
			var idx int
			fmt.Sscanf(strings.TrimSpace(idxStr), "%d", &idx)
			if idx > 0 && idx <= len(volumes) {
				vol := volumes[idx-1]
				fmt.Printf("%s正在删除卷 %s...%s\n", ColorYellow, vol.Name, ColorNC)
				if err := vol.Remove(false); err != nil {
					fmt.Printf("%s失败: %v%s\n", ColorRed, err, ColorNC)
				} else {
					fmt.Printf("%s成功%s\n", ColorGreen, ColorNC)
				}
			} else {
				fmt.Printf("%s无效的索引%s\n", ColorRed, ColorNC)
			}
		}
	}
}
