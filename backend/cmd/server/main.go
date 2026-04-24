package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"glipz.io/backend/internal/config"
	"glipz.io/backend/internal/httpserver"
	"glipz.io/backend/internal/migrate"
	"glipz.io/backend/internal/s3client"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("postgres ping: %v", err)
	}
	if err := migrate.Run(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := migrate.RunSocial(ctx, pool); err != nil {
		log.Fatalf("migrate social: %v", err)
	}
	if err := migrate.RunProfile(ctx, pool); err != nil {
		log.Fatalf("migrate profile: %v", err)
	}
	if err := migrate.RunUserBadges(ctx, pool); err != nil {
		log.Fatalf("migrate user badges: %v", err)
	}
	if err := migrate.RunFollow(ctx, pool); err != nil {
		log.Fatalf("migrate follow: %v", err)
	}
	if err := migrate.RunNSFW(ctx, pool); err != nil {
		log.Fatalf("migrate nsfw: %v", err)
	}
	if err := migrate.RunNotifications(ctx, pool); err != nil {
		log.Fatalf("migrate notifications: %v", err)
	}
	if err := migrate.RunPostsExtras(ctx, pool); err != nil {
		log.Fatalf("migrate posts extras: %v", err)
	}
	if err := migrate.RunFederationSubscribers(ctx, pool); err != nil {
		log.Fatalf("migrate federation subscribers: %v", err)
	}
	if err := migrate.RunFederationIncoming(ctx, pool); err != nil {
		log.Fatalf("migrate federation incoming: %v", err)
	}
	if err := migrate.RunBookmarks(ctx, pool); err != nil {
		log.Fatalf("migrate bookmarks: %v", err)
	}
	if err := migrate.RunModeration(ctx, pool); err != nil {
		log.Fatalf("migrate moderation: %v", err)
	}
	if err := migrate.RunSearchTags(ctx, pool); err != nil {
		log.Fatalf("migrate search tags: %v", err)
	}
	if err := migrate.RunFederationRemoteFollow(ctx, pool); err != nil {
		log.Fatalf("migrate federation remote follow: %v", err)
	}
	if err := migrate.RunFederationDeliveries(ctx, pool); err != nil {
		log.Fatalf("migrate federation deliveries: %v", err)
	}
	if err := migrate.RunFederationActions(ctx, pool); err != nil {
		log.Fatalf("migrate federation actions: %v", err)
	}
	if err := migrate.RunFederationDomainBlock(ctx, pool); err != nil {
		log.Fatalf("migrate federation domain block: %v", err)
	}
	if err := migrate.RunFederationUserPrivacy(ctx, pool); err != nil {
		log.Fatalf("migrate federation user privacy: %v", err)
	}
	if err := migrate.RunFederationKnownInstances(ctx, pool); err != nil {
		log.Fatalf("migrate federation known instances: %v", err)
	}
	if err := migrate.RunFederationRemoteCustomEmojis(ctx, pool); err != nil {
		log.Fatalf("migrate federation remote custom emojis: %v", err)
	}
	if err := migrate.RunOAuthAPI(ctx, pool); err != nil {
		log.Fatalf("migrate oauth api: %v", err)
	}
	if err := migrate.RunPendingRegistrations(ctx, pool); err != nil {
		log.Fatalf("migrate pending registrations: %v", err)
	}
	if err := migrate.RunLiveSpaceCleanup(ctx, pool); err != nil {
		log.Fatalf("migrate live space cleanup: %v", err)
	}
	if err := migrate.RunDirectMessages(ctx, pool); err != nil {
		log.Fatalf("migrate direct messages: %v", err)
	}
	if err := migrate.RunFederationDM(ctx, pool); err != nil {
		log.Fatalf("migrate federation dm: %v", err)
	}
	if err := migrate.RunDMCallPolicyColumns(ctx, pool); err != nil {
		log.Fatalf("migrate dm_call_policy: %v", err)
	}
	if err := migrate.RunWebPush(ctx, pool); err != nil {
		log.Fatalf("migrate web push: %v", err)
	}
	if err := migrate.RunFanclubPatreon(ctx, pool); err != nil {
		log.Fatalf("migrate fanclub patreon: %v", err)
	}
	if err := migrate.RunFederationIncomingMembership(ctx, pool); err != nil {
		log.Fatalf("migrate federation incoming membership: %v", err)
	}

	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis url: %v", err)
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis ping: %v", err)
	}

	s3c, err := s3client.New(
		cfg.S3Endpoint,
		cfg.S3PublicEndpoint,
		cfg.S3Region,
		cfg.S3AccessKey,
		cfg.S3SecretKey,
		cfg.S3Bucket,
		cfg.S3UsePathStyle,
	)
	if err != nil {
		log.Fatalf("s3: %v", err)
	}

	h := httpserver.New(cfg, pool, rdb, s3c)
	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
