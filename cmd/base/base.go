package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/fivecode/plotty/core/config"
	"github.com/fivecode/plotty/internal/infrastructure/postgres"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	sharedrmq "github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

var seededSlugs = []string{
	"hp-vosmoy-kurs",
	"hp-hranitel-zapretnoy-sekcii",
	"hp-zelye-zabytogo-leta",
	"hp-kapitan-zapasnogo-sostava",
	"hp-karta-kotoroy-ne-bylo",
	"hp-severnaya-bashnya",
	"witcher-kontrakt-na-bolote",
	"witcher-naydyonysh",
	"witcher-bard-bez-golosa",
	"witcher-znak-na-ladoni",
	"witcher-aptekarsha-iz-oksenfurta",
	"lotr-brod-cherez-bruinen",
	"lotr-sad-posle-pohoda",
	"lotr-pesn-o-dvuh-derevyah",
	"lotr-gnom-i-zvyozdnaya-karta",
	"naruto-svitok-bez-tehnik",
	"naruto-komanda-iz-troih",
	"naruto-chaynaya-na-granice",
	"naruto-pechat-na-serdce",
	"naruto-ekzamen-na-stoykost",
	"marvel-stazhyorka-stark-industries",
	"marvel-tihiy-geroy",
	"marvel-nochnaya-smena",
	"marvel-laboratoriya-nomer-sem",
	"marvel-pisma-strazham",
	"dc-signal-nad-gorodom",
	"dc-tot-kto-chinit",
	"dc-muzey-v-polnoch",
	"dc-malenkiy-gorod-bolshoy-strah",
	"sherlock-delo-o-pustoy-kvartire",
	"sherlock-kvartirantka-s-beyker-strit",
	"sherlock-skripka-v-tumane",
	"sherlock-poslednee-delo-doktora",
	"sw-mehanik-s-vneshнего-kolca",
	"sw-padavan-bez-mecha",
	"sw-kontrabandistka-s-sovestyu",
	"sw-golos-v-efire",
	"got-meyster-zamyorzshego-zamka",
	"got-pesn-o-malom-dome",
	"got-ta-chto-chinit-znamyona",
	"got-voron-s-durnoy-vestyu",
	"aot-za-stenoy",
	"aot-garnizon-u-vorot",
	"aot-kartograf-zapretnyh-zemel",
	"aot-posledniy-rubezh",
	"orig-mayak-na-krayu-karty",
	"orig-chasovshchik-chto-chinil-vremya",
	"orig-pisma-iz-niotkuda",
	"orig-hranitelnica-posledнего-sada",
	"orig-devochka-i-kit",
}

type storyRow struct {
	id         uuid.UUID
	slug       string
	fandomSlug string
}

type chapterRow struct {
	id        uuid.UUID
	storyID   uuid.UUID
	content   string
	createdAt time.Time
}

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pool, err := postgres.NewPostgresPool(ctx, cfg.GetDSN())
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	rmqConn, err := rabbitmq.NewConnection(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("rabbitmq connect: %v", err)
	}
	defer rmqConn.Close()

	ch, err := rmqConn.Channel()
	if err != nil {
		log.Fatalf("rabbitmq channel: %v", err)
	}
	defer ch.Close()

	ch.QueueDeclare("ml_tasks_queue", true, false, false, false, nil)
	ch.QueueDeclare("ml_image_queue", true, false, false, false, nil) // Декларация новой очереди в сидинге

	stories, err := loadStories(ctx, pool)
	if err != nil {
		log.Fatalf("load stories: %v", err)
	}
	log.Printf("Loaded %d seeded stories", len(stories))

	loreTotal, imgTotal, summaryTotal := 0, 0, 0

	for _, st := range stories {
		chapters, err := loadChapters(ctx, pool, st.id)
		if err != nil {
			log.Printf("SKIP story %s: load chapters: %v", st.slug, err)
			continue
		}
		if len(chapters) == 0 {
			log.Printf("SKIP story %s: no chapters", st.slug)
			continue
		}

		var prevID string
		for _, c := range chapters {
			if err := publishExtractLore(ctx, ch, st.id, c, prevID); err != nil {
				log.Printf("  lore ERR story=%s chapter=%s: %v", st.slug, c.id, err)
			} else {
				loreTotal++
			}
			prevID = c.id.String()
		}
		log.Printf("  [lore] %s: %d chapters queued", st.slug, len(chapters))

		first := chapters[0]
		if err := publishGenerateSummary(ctx, ch, st.id, first); err != nil {
			log.Printf("  summary ERR story=%s: %v", st.slug, err)
		} else {
			summaryTotal++
			log.Printf("  [summary] %s: queued", st.slug)
		}

		if st.fandomSlug != "" {
			if err := publishImageGen(ctx, pool, ch, st, first); err != nil {
				log.Printf("  img ERR story=%s: %v", st.slug, err)
			} else {
				imgTotal++
				log.Printf("  [img]  %s: queued", st.slug)
			}
		}
	}

	log.Printf("Done. extract_lore: %d, generate_summary: %d, image_gen: %d", loreTotal, summaryTotal, imgTotal)
}

func loadStories(ctx context.Context, pool *pgxpool.Pool) ([]storyRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.id, s.slug,
		       COALESCE((
		           SELECT t.slug FROM story_tags st
		           JOIN tags t ON t.id = st.tag_id
		           WHERE st.story_id = s.id AND t.category = 'directionality' AND t.slug != 'originals'
		           LIMIT 1
		       ), '') AS fandom_slug
		FROM stories s
		WHERE s.slug = ANY($1)
	`, seededSlugs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []storyRow
	for rows.Next() {
		var r storyRow
		if err := rows.Scan(&r.id, &r.slug, &r.fandomSlug); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func loadChapters(ctx context.Context, pool *pgxpool.Pool, storyID uuid.UUID) ([]chapterRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, story_id, content, created_at
		FROM chapters
		WHERE story_id = $1 AND status = 'published'
		ORDER BY created_at ASC
	`, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []chapterRow
	for rows.Next() {
		var c chapterRow
		if err := rows.Scan(&c.id, &c.storyID, &c.content, &c.createdAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func publishGenerateSummary(ctx context.Context, ch *amqp.Channel, storyID uuid.UUID, first chapterRow) error {
	task := sharedrmq.MLTaskMessage{
		TaskID:  uuid.NewString(),
		TraceID: uuid.NewString(),
		Type:    "generate_summary",
		Payload: first.content,
		Metadata: map[string]string{
			"story_id": storyID.String(),
		},
	}
	return publish(ctx, ch, "ml_tasks_queue", task)
}

func publishExtractLore(ctx context.Context, ch *amqp.Channel, storyID uuid.UUID, c chapterRow, prevChapterID string) error {
	task := sharedrmq.MLTaskMessage{
		TaskID:  uuid.NewString(),
		TraceID: uuid.NewString(),
		Type:    "extract_lore",
		Payload: c.content,
		Metadata: map[string]string{
			"story_id":        storyID.String(),
			"chapter_id":      c.id.String(),
			"prev_chapter_id": prevChapterID,
		},
	}
	return publish(ctx, ch, "ml_tasks_queue", task)
}

type imageGenPayload struct {
	ChapterID string `json:"chapterId"`
	Content   string `json:"content"`
	Prompt    string `json:"prompt"`
}

func publishImageGen(ctx context.Context, pool *pgxpool.Pool, ch *amqp.Channel, st storyRow, first chapterRow) error {
	jobID := uuid.New()
	now := time.Now().UTC()

	payload, _ := json.Marshal(imageGenPayload{
		ChapterID: first.id.String(),
		Content:   first.content,
		Prompt:    "обложка для фанфика",
	})

	_, err := pool.Exec(ctx, `
		INSERT INTO ai_jobs (id, type, status, story_id, chapter_id, input_payload, created_at, updated_at)
		VALUES ($1, 'image_generation', 'processing', $2, $3, $4, $5, $5)
		ON CONFLICT (id) DO NOTHING
	`, jobID, st.id, first.id, payload, now)
	if err != nil {
		return fmt.Errorf("insert ai_job: %w", err)
	}

	task := sharedrmq.MLTaskMessage{
		TaskID:  jobID.String(),
		TraceID: uuid.NewString(),
		Type:    "image_gen",
		Payload: string(payload),
	}
	return publish(ctx, ch, "ml_image_queue", task) // Публикуем в новую изолированную очередь
}

func publish(ctx context.Context, ch *amqp.Channel, queue string, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return ch.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}
