<?php

namespace Tests\Feature;

use Tests\TestCase;

class HealthEndpointsTest extends TestCase
{
    public function test_live_endpoint_returns_up_response(): void
    {
        $response = $this->getJson('/api/health/live');

        $response->assertStatus(200)
            ->assertJsonStructure([
                'status',
                'service',
                'timestamp',
            ]);
    }

    public function test_ready_endpoint_returns_structured_payload(): void
    {
        $response = $this->getJson('/api/health/ready');

        $this->assertContains($response->getStatusCode(), [200, 503]);

        $response->assertJsonStructure([
            'status',
            'service',
            'dependencies' => ['mongodb', 'redis_query', 'redis_cache'],
            'timestamp',
        ]);
    }
}
