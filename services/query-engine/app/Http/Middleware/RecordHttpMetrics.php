<?php

namespace App\Http\Middleware;

use App\Support\PrometheusMetrics;
use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

class RecordHttpMetrics
{
    public function handle(Request $request, Closure $next): Response
    {
        $start = microtime(true);
        /** @var Response $response */
        $response = $next($request);

        $path = trim($request->path(), '/');
        if ($path !== 'metrics') {
            $durationSeconds = microtime(true) - $start;
            PrometheusMetrics::recordRequest($response->getStatusCode(), $durationSeconds);
        }

        return $response;
    }
}
