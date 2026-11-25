```mermaid
flowchart TB
    classDef server fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef db fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    classDef cache fill:#fff3e0,stroke:#ef6c00,stroke-width:2px
    classDef mq fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef proxy fill:#eceff1,stroke:#455a64,stroke-width:2px
    classDef ui fill:#fce4ec,stroke:#880e4f,stroke-width:2px

    subgraph Users ["Users"]
        Admin[(Admin)]:::ui
        Editor[(Editor)]:::ui
        Viewer[(Viewer)]:::ui
        Operator[(Operator)]:::ui
    end

    subgraph Edge ["Edge"]
        Nginx[Nginx<br/>TLS termination + CORS + rate limit]:::proxy
    end

    subgraph App ["Application Containers"]
        Server[IntegrationHub Server<br/>Go 1.22 + Chi<br/>API + executor + scheduler + webhook + connector]:::server
        Web[Admin UI<br/>React + TS + Tailwind + shadcn/ui]:::ui
    end

    subgraph Data ["Data Layer"]
        Postgres[(PostgreSQL 16<br/>config / metadata / ACID<br/>pgx/v5 + JSONB)]:::db
        Mongo[(MongoDB 7<br/>response data / audit /<br/>schema-flexible documents)]:::db
        Redis[(Redis 7<br/>rate limit / session / hot<br/>cache + go-redis/v9)]:::cache
        NATS[(NATS JetStream<br/>execution events / DLQ /<br/>at-least-once)]:::mq
    end

    Users -.->|HTTPS| Nginx
    Nginx -->|route| Server
    Nginx -->|serve| Web
    Server -->|SQL| Postgres
    Server -->|BSON| Mongo
    Server -->|Redis| Redis
    Server -->|NATS| NATS
    Server -.->|HTTP(S)| ThirdPartyAPI["3rd Party APIs"]
    ThirdPartyAPI -.->|webhook| Server
```

**C4 Container Diagram.** Containers, technologies, and high-level communication paths.
