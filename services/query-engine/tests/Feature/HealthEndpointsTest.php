<?php

namespace Tests\Feature;

use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Redis;
use Mockery;
use Tests\TestCase;

class HealthEndpointsTest extends TestCase
{
    protected function tearDown(): void
    {
        Mockery::close();
        parent::tearDown();
    }

    public function test_live_endpoint_returns_up_response(): void
    {
        $response = $this->getJson('/api/health/live');

        $response->assertStatus(200)
            ->assertJsonPath('status', 'up')
            ->assertJsonPath('service', 'query-engine')
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

        if ($response->getStatusCode() === 200) {
            $response->assertJsonPath('status', 'ready');
        } else {
            $response->assertJsonPath('status', 'not_ready');
        }
    }

    public function test_ready_endpoint_returns_503_when_dependencies_fail(): void
    {
        DB::shouldReceive('connection')
            ->once()
            ->with('mongodb')
            ->andThrow(new \RuntimeException('mongo unavailable'));

        Redis::shouldReceive('connection')
            ->once()
            ->with('default')
            ->andThrow(new \RuntimeException('query redis unavailable'));

        Redis::shouldReceive('connection')
            ->once()
            ->with('cache')
            ->andThrow(new \RuntimeException('cache redis unavailable'));

        $response = $this->getJson('/api/health/ready');

        $response->assertStatus(503)
            ->assertJsonPath('status', 'not_ready')
            ->assertJsonPath('service', 'query-engine')
            ->assertJsonPath('dependencies.mongodb', false)
            ->assertJsonPath('dependencies.redis_query', false)
            ->assertJsonPath('dependencies.redis_cache', false);
    }
}
