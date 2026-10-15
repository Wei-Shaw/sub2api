package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

// 手动重置密码工具
// 使用方法: go run reset-password-manual.go "新密码"

func main() {
	if len(os.Args) != 2 {
		fmt.Println("使用方法: go run reset-password-manual.go <新密码>")
		fmt.Println("示例: go run reset-password-manual.go MyNewPass123!")
		os.Exit(1)
	}

	password := os.Args[1]

	// 使用与系统相同的bcrypt加密
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Printf("加密失败: %v\n", err)
		os.Exit(1)
	}

	hashedPassword := string(hashedBytes)

	fmt.Println("=====================================")
	fmt.Println("密码Hash生成成功！")
	fmt.Println("=====================================")
	fmt.Println("新密码:", password)
	fmt.Println("Hash值:", hashedPassword)
	fmt.Println("=====================================")
	fmt.Println("\n执行以下SQL更新用户密码:")
	fmt.Println("UPDATE users SET password_hash = '" + hashedPassword + "' WHERE email = '用户邮箱';")
	fmt.Println("\n重要提示:")
	fmt.Println("1. 替换'用户邮箱'为实际的用户邮箱地址")
	fmt.Println("2. 执行后该用户的所有现有token将失效")
	fmt.Println("3. 建议用户登录后立即修改密码")
}
