<?php

namespace Tests\Feature;

use Tests\TestCase;

class MetricsEndpointTest extends TestCase
{
    public function test_metrics_endpoint_returns_prometheus_payload(): void
    {
        $response = $this->get('/metrics');

        $response->assertStatus(200);
        $response->assertHeader('Content-Type', 'text/plain; version=0.0.4; charset=utf-8');
        $response->assertSee('http_requests_total', false);
        $response->assertSee('http_request_duration_seconds_bucket', false);
    }
}
