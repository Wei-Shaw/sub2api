ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_endpoint_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_endpoint_check
    CHECK (endpoint IN ('any', 'messages', 'count_tokens', 'responses',
                        'chat_completions', 'embeddings', 'decisions', 'images', 'gemini'));
