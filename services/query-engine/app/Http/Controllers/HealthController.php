<?php

namespace App\Http\Controllers;

use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Redis;
use Throwable;

class HealthController extends Controller
{
    public function live()
    {
        return response()->json([
            'status' => 'up',
            'service' => 'query-engine',
            'timestamp' => now()->toIso8601String(),
        ]);
    }

    public function ready()
    {
        $dependencies = [
            'mongodb' => false,
            'redis_query' => false,
            'redis_cache' => false,
        ];

        try {
            DB::connection('mongodb')->table('metadata')->limit(1)->get();
            $dependencies['mongodb'] = true;
        } catch (Throwable $e) {
            $dependencies['mongodb'] = false;
        }

        try {
            Redis::connection('default')->ping();
            $dependencies['redis_query'] = true;
        } catch (Throwable $e) {
            $dependencies['redis_query'] = false;
        }

        try {
            Redis::connection('cache')->ping();
            $dependencies['redis_cache'] = true;
        } catch (Throwable $e) {
            $dependencies['redis_cache'] = false;
        }

        $isReady =
            $dependencies['mongodb'] &&
            $dependencies['redis_query'] &&
            $dependencies['redis_cache'];

        return response()->json([
            'status' => $isReady ? 'ready' : 'not_ready',
            'service' => 'query-engine',
            'dependencies' => $dependencies,
            'timestamp' => now()->toIso8601String(),
        ], $isReady ? 200 : 503);
    }
}
