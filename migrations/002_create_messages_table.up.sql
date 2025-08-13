CREATE TABLE messages (
    id UUID DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    payload JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, tenant_id)
) PARTITION BY LIST (tenant_id);

CREATE INDEX idx_messages_tenant_id_created_at ON messages(tenant_id, created_at DESC);
CREATE INDEX idx_messages_created_at ON messages(created_at DESC);