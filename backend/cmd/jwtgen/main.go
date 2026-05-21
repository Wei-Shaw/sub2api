package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func main() {
	email := flag.String("email", "", "Admin email to issue a JWT for (defaults to first active admin)")
	flag.Parse()

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	client, sqlDB, err := repository.InitEnt(cfg)
	if err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("failed to close db: %v", err)
		}
	}()

	userRepo := repository.NewUserRepository(client, sqlDB)
	authService := service.NewAuthService(client, userRepo, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user *service.User
	if *email != "" {
		user, err = userRepo.GetByEmail(ctx, *email)
	} else {
		user, err = userRepo.GetFirstAdmin(ctx)
	}
	if err != nil {
		log.Fatalf("failed to resolve admin user: %v", err)
	}

	token, err := authService.GenerateToken(user)
	if err != nil {
		log.Fatalf("failed to generate token: %v", err)
	}

	fmt.Printf("ADMIN_EMAIL=%s\nADMIN_USER_ID=%d\nJWT=%s\n", user.Email, user.ID, token)
}
