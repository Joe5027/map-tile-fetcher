using System.Collections.Concurrent;
using System.IO.Compression;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

var builder = WebApplication.CreateBuilder(args);
builder.Services.AddSingleton<TileDownloadManager>();
builder.Services.AddHttpClient("tiles", client =>
{
    client.Timeout = TimeSpan.FromSeconds(30);
    client.DefaultRequestHeaders.UserAgent.Add(new ProductInfoHeaderValue("TianDiTuDownLoaderWeb", "1.0"));
});

var app = builder.Build();
app.UseDefaultFiles();
app.UseStaticFiles();

app.MapGet("/api/config", (IConfiguration config) =>
{
    return Results.Ok(new DownloadRequest
    {
        Token = config["TIANDITU_TOKEN"] ?? "",
        Layers = LayerCatalog.DefaultKeys,
        LeftUpLon = ReadDouble(config["LEFT_UP_LON"], 109.704612),
        LeftUpLat = ReadDouble(config["LEFT_UP_LAT"], 39.65844),
        RightDownLon = ReadDouble(config["RIGHT_DOWN_LON"], 109.922622),
        RightDownLat = ReadDouble(config["RIGHT_DOWN_LAT"], 39.50497),
        MaxLayer = ReadInt(config["MAX_LAYER"], 18),
        DownloadPath = config["DOWNLOAD_PATH"] ?? "/data/tiles"
    });
});

app.MapGet("/api/jobs", (TileDownloadManager manager) => Results.Ok(manager.GetAllStatuses()));
app.MapGet("/api/jobs/current", (TileDownloadManager manager) => Results.Ok(manager.GetCurrentStatus()));
app.MapGet("/api/jobs/{jobId}", (string jobId, TileDownloadManager manager) => manager.GetStatus(jobId) is { } status ? Results.Ok(status) : Results.NotFound());

app.MapPost("/api/jobs", async (DownloadRequest request, TileDownloadManager manager) =>
{
    var validation = request.Validate();
    if (validation is not null)
    {
        return Results.BadRequest(new { error = validation });
    }

    return Results.Ok(await manager.StartAsync(request));
});

app.MapPost("/api/jobs/{jobId}/stop", (string jobId, TileDownloadManager manager) => Results.Ok(manager.Stop(jobId)));
app.MapPost("/api/jobs/{jobId}/resume", (string jobId, TileDownloadManager manager) => Results.Ok(manager.Resume(jobId)));
app.MapPost("/api/jobs/{jobId}/layers/{layer}/stop", (string jobId, string layer, TileDownloadManager manager) => Results.Ok(manager.StopLayer(jobId, layer)));
app.MapPost("/api/jobs/{jobId}/layers/{layer}/resume", (string jobId, string layer, TileDownloadManager manager) => Results.Ok(manager.ResumeLayer(jobId, layer)));
app.MapPost("/api/jobs/{jobId}/layers/{layer}/retry", (string jobId, string layer, TileDownloadManager manager) => Results.Ok(manager.RetryLayer(jobId, layer)));
app.MapDelete("/api/jobs/{jobId}", (string jobId, TileDownloadManager manager) =>
{
    return manager.Delete(jobId)
        ? Results.Ok(new { deleted = true })
        : Results.NotFound(new { error = "任务不存在或无法删除。" });
});

app.MapGet("/api/jobs/{jobId}/layers/{layer}/failures", (string jobId, string layer, TileDownloadManager manager) =>
{
    var file = manager.GetFailureRecordPath(jobId, layer);
    return file is null || !File.Exists(file)
        ? Results.NotFound(new { error = "当前图层没有失败记录文件。" })
        : Results.File(file, "application/json", $"{jobId}-{layer}-RecorderList.json");
});

app.MapGet("/api/jobs/{jobId}/archive", (string jobId, TileDownloadManager manager) =>
{
    var archive = manager.CreateJobArchive(jobId);
    return archive is null
        ? Results.NotFound(new { error = "没有可下载的任务产物。" })
        : Results.File(File.OpenRead(archive.Value.Path), "application/gzip", archive.Value.FileName);
});

app.MapGet("/api/jobs/{jobId}/layers/{layer}/archive", (string jobId, string layer, TileDownloadManager manager) =>
{
    var archive = manager.CreateLayerArchive(jobId, layer);
    return archive is null
        ? Results.NotFound(new { error = "没有可下载的子任务产物。" })
        : Results.File(File.OpenRead(archive.Value.Path), "application/gzip", archive.Value.FileName);
});

app.MapGet("/api/tiles/{layer}/{z:int}/{x:int}/{y:int}", async (string layer, int z, int x, int y, string? tk, IHttpClientFactory httpClientFactory, IConfiguration config, HttpContext context) =>
{
    if (!LayerCatalog.TryGet(layer, out _))
    {
        return Results.NotFound();
    }

    var token = string.IsNullOrWhiteSpace(tk) ? config["TIANDITU_TOKEN"] ?? "" : tk;
    var cacheRoot = Path.Combine(config["DOWNLOAD_PATH"] ?? "/data/tiles", ".preview-cache", layer, z.ToString(), x.ToString());
    var cacheFile = Path.Combine(cacheRoot, $"{y}.tile");
    var cacheTypeFile = Path.Combine(cacheRoot, $"{y}.content-type");
    if (File.Exists(cacheFile))
    {
        var cachedContentType = File.Exists(cacheTypeFile) ? await File.ReadAllTextAsync(cacheTypeFile) : "image/jpeg";
        context.Response.Headers.CacheControl = "public,max-age=86400";
        return Results.File(File.OpenRead(cacheFile), cachedContentType);
    }

    var server = Math.Abs(HashCode.Combine(layer, z, x, y)) % 8;
    var url = $"http://t{server}.tianditu.gov.cn/{layer}_w/wmts?SERVICE=WMTS&REQUEST=GetTile&VERSION=1.0.0&LAYER={layer}&STYLE=default&TILEMATRIXSET=w&FORMAT=tiles&TILEMATRIX={z}&TILEROW={y}&TILECOL={x}&tk={token}";

    try
    {
        var response = await httpClientFactory.CreateClient("tiles").GetAsync(url);
        if (!response.IsSuccessStatusCode || response.Content.Headers.ContentType?.MediaType?.StartsWith("text/", StringComparison.OrdinalIgnoreCase) == true)
        {
            return Results.NotFound();
        }

        var contentType = response.Content.Headers.ContentType?.ToString() ?? "image/png";
        var bytes = await response.Content.ReadAsByteArrayAsync();
        Directory.CreateDirectory(cacheRoot);
        await File.WriteAllBytesAsync(cacheFile, bytes);
        await File.WriteAllTextAsync(cacheTypeFile, contentType);
        context.Response.Headers.CacheControl = "public,max-age=86400";
        return Results.File(bytes, contentType);
    }
    catch
    {
        return Results.NotFound();
    }
});

app.Run();

static double ReadDouble(string? value, double fallback) => double.TryParse(value, out var result) ? result : fallback;
static int ReadInt(string? value, int fallback) => int.TryParse(value, out var result) ? result : fallback;

public sealed class TileDownloadManager
{
    private readonly IHttpClientFactory httpClientFactory;
    private readonly object gate = new();
    private readonly Dictionary<string, MultiLayerTileJob> liveJobs = new(StringComparer.OrdinalIgnoreCase);
    private readonly Dictionary<string, JobStatus> storedStatuses = new(StringComparer.OrdinalIgnoreCase);
    private string? currentJobId;

    public TileDownloadManager(IHttpClientFactory httpClientFactory)
    {
        this.httpClientFactory = httpClientFactory;
        LoadStoredStatuses();
    }

    public Task<JobStatus> StartAsync(DownloadRequest request)
    {
        lock (gate)
        {
            var job = MultiLayerTileJob.Create(request, httpClientFactory, SaveStatus);
            liveJobs[job.JobId] = job;
            currentJobId = job.JobId;
            job.Start();
            var status = job.GetStatus();
            SaveStatus(status);
            return Task.FromResult(status);
        }
    }

    public List<JobStatus> GetAllStatuses()
    {
        lock (gate)
        {
            foreach (var job in liveJobs.Values)
            {
                SaveStatus(job.GetStatus());
            }

            return storedStatuses.Values
                .OrderByDescending(status => status.CreatedAt)
                .ToList();
        }
    }

    public JobStatus? GetCurrentStatus()
    {
        lock (gate)
        {
            return currentJobId is null ? null : GetStatus(currentJobId);
        }
    }

    public JobStatus? GetStatus(string jobId)
    {
        lock (gate)
        {
            if (liveJobs.TryGetValue(jobId, out var job))
            {
                var status = job.GetStatus();
                SaveStatus(status);
                return status;
            }

            return storedStatuses.TryGetValue(jobId, out var stored) ? stored : null;
        }
    }

    public JobStatus? Stop(string jobId)
    {
        lock (gate)
        {
            if (liveJobs.TryGetValue(jobId, out var job))
            {
                job.StopAll();
                var status = job.GetStatus();
                SaveStatus(status);
                return status;
            }

            return GetStatus(jobId);
        }
    }

    public JobStatus? Resume(string jobId)
    {
        lock (gate)
        {
            if (!liveJobs.TryGetValue(jobId, out var job) && !TryRestoreJob(jobId, out job))
            {
                return GetStatus(jobId);
            }

            if (job is not null)
            {
                job.ResumeAll();
                var status = job.GetStatus();
                SaveStatus(status);
                return status;
            }

            return null;
        }
    }

    public JobStatus? StopLayer(string jobId, string layer)
    {
        lock (gate)
        {
            if (liveJobs.TryGetValue(jobId, out var job))
            {
                job.StopLayer(layer);
                var status = job.GetStatus();
                SaveStatus(status);
                return status;
            }

            return GetStatus(jobId);
        }
    }

    public JobStatus? ResumeLayer(string jobId, string layer)
    {
        lock (gate)
        {
            if (!liveJobs.TryGetValue(jobId, out var job) && !TryRestoreJob(jobId, out job))
            {
                return GetStatus(jobId);
            }

            if (job is not null)
            {
                job.ResumeLayer(layer);
                var status = job.GetStatus();
                SaveStatus(status);
                return status;
            }

            return null;
        }
    }

    public JobStatus? RetryLayer(string jobId, string layer)
    {
        lock (gate)
        {
            if (liveJobs.TryGetValue(jobId, out var job))
            {
                job.RetryLayer(layer);
                var status = job.GetStatus();
                SaveStatus(status);
                return status;
            }

            return GetStatus(jobId);
        }
    }

    public bool Delete(string jobId)
    {
        lock (gate)
        {
            if (liveJobs.Remove(jobId, out var liveJob))
            {
                liveJob.StopAll();
            }

            if (!storedStatuses.Remove(jobId, out var status))
            {
                return false;
            }

            if (currentJobId?.Equals(jobId, StringComparison.OrdinalIgnoreCase) == true)
            {
                currentJobId = storedStatuses.Values.OrderByDescending(item => item.CreatedAt).FirstOrDefault()?.JobId;
            }

            DeleteJobDirectory(status.OutputPath);
            return true;
        }
    }

    public string? GetFailureRecordPath(string jobId, string layer)
    {
        lock (gate)
        {
            if (liveJobs.TryGetValue(jobId, out var job))
            {
                return job.GetFailureRecordPath(layer);
            }

            return GetStatus(jobId)?.Layers.FirstOrDefault(item => item.Key.Equals(layer, StringComparison.OrdinalIgnoreCase))?.FailureRecordPath;
        }
    }

    public ArchiveInfo? CreateJobArchive(string jobId)
    {
        var status = GetStatus(jobId);
        if (status is null || !Directory.Exists(status.OutputPath))
        {
            return null;
        }

        var fileName = $"{Initials.Task}.tar.gz";
        var path = Path.Combine(status.ArtifactPath, fileName);
        TarGz.CreateFromDirectory(status.OutputPath, path, "task", excludeDirectoryNames: new[] { "artifacts" });
        UpdateArtifact(jobId, null, fileName, path);
        return new ArchiveInfo(path, fileName);
    }

    public ArchiveInfo? CreateLayerArchive(string jobId, string layer)
    {
        var status = GetStatus(jobId);
        var layerStatus = status?.Layers.FirstOrDefault(item => item.Key.Equals(layer, StringComparison.OrdinalIgnoreCase));
        if (status is null || layerStatus is null || !Directory.Exists(layerStatus.OutputPath))
        {
            return null;
        }

        var fileName = $"{Initials.Task}{Initials.ForLayer(layerStatus.Key)}.tar.gz";
        var path = Path.Combine(status.ArtifactPath, fileName);
        TarGz.CreateFromDirectory(layerStatus.OutputPath, path, layerStatus.Key);
        UpdateArtifact(jobId, layerStatus.Key, fileName, path);
        return new ArchiveInfo(path, fileName);
    }

    private void SaveStatus(JobStatus status)
    {
        if (storedStatuses.TryGetValue(status.JobId, out var existing))
        {
            var layers = status.Layers.Select(layer =>
            {
                var oldLayer = existing.Layers.FirstOrDefault(item => item.Key.Equals(layer.Key, StringComparison.OrdinalIgnoreCase));
                return layer.Artifact is null && oldLayer?.Artifact is not null ? layer with { Artifact = oldLayer.Artifact } : layer;
            }).ToList();
            status = status with
            {
                Artifact = status.Artifact ?? existing.Artifact,
                CompletedAt = status.CompletedAt,
                Layers = layers
            };
        }

        storedStatuses[status.JobId] = status;
        Directory.CreateDirectory(status.OutputPath);
        Directory.CreateDirectory(status.ArtifactPath);
        File.WriteAllText(MetadataPath(status.OutputPath), JsonSerializer.Serialize(status, JsonOptions.Default));
    }

    private void UpdateArtifact(string jobId, string? layer, string fileName, string path)
    {
        lock (gate)
        {
            if (!storedStatuses.TryGetValue(jobId, out var status))
            {
                return;
            }

            var artifact = new ArtifactInfo(fileName, path, DateTimeOffset.Now);
            if (layer is null)
            {
                status = status with { Artifact = artifact };
            }
            else
            {
                var layers = status.Layers.Select(item => item.Key.Equals(layer, StringComparison.OrdinalIgnoreCase) ? item with { Artifact = artifact } : item).ToList();
                status = status with { Layers = layers };
            }

            SaveStatus(status);
        }
    }

    private void LoadStoredStatuses()
    {
        if (!Directory.Exists("/data/tiles"))
        {
            return;
        }

        foreach (var metadata in Directory.EnumerateFiles("/data/tiles", "metadata.json", SearchOption.AllDirectories))
        {
            try
            {
                var status = JsonSerializer.Deserialize<JobStatus>(File.ReadAllText(metadata), JsonOptions.Default);
                if (status is not null)
                {
                    storedStatuses[status.JobId] = status;
                    currentJobId ??= status.JobId;
                }
            }
            catch
            {
            }
        }
    }

    private bool TryRestoreJob(string jobId, out MultiLayerTileJob? job)
    {
        job = null;
        if (!storedStatuses.TryGetValue(jobId, out var status) || status.State != nameof(JobState.Stopped))
        {
            return false;
        }

        job = MultiLayerTileJob.RestoreForResume(status, httpClientFactory, SaveStatus);
        liveJobs[job.JobId] = job;
        return true;
    }

    private static void DeleteJobDirectory(string outputPath)
    {
        var fullPath = Path.GetFullPath(outputPath);
        var allowedRoot = Path.GetFullPath("/data/tiles/tasks");
        if (!fullPath.StartsWith(allowedRoot, StringComparison.OrdinalIgnoreCase) || !Directory.Exists(fullPath))
        {
            return;
        }

        for (var attempt = 0; attempt < 3; attempt++)
        {
            try
            {
                Directory.Delete(fullPath, recursive: true);
                return;
            }
            catch
            {
                Thread.Sleep(150);
            }
        }
    }

    private static string MetadataPath(string outputPath) => Path.Combine(outputPath, "metadata.json");
}

public sealed class MultiLayerTileJob
{
    private readonly Dictionary<string, LayerTileJob> layerJobs;
    private readonly Action<JobStatus> statusChanged;

    private MultiLayerTileJob(string jobId, string name, DownloadRequest request, string outputPath, Dictionary<string, LayerTileJob> layerJobs, Action<JobStatus> statusChanged, DateTimeOffset? createdAt = null)
    {
        JobId = jobId;
        Name = name;
        Request = request;
        OutputPath = outputPath;
        ArtifactPath = Path.Combine(outputPath, "artifacts");
        this.layerJobs = layerJobs;
        this.statusChanged = statusChanged;
        CreatedAt = createdAt ?? DateTimeOffset.Now;
    }

    public string JobId { get; }
    public string Name { get; }
    public DownloadRequest Request { get; }
    public string OutputPath { get; }
    public string ArtifactPath { get; }
    public DateTimeOffset CreatedAt { get; }
    public bool IsRunning => layerJobs.Values.Any(job => job.IsRunning);

    public static MultiLayerTileJob Create(DownloadRequest request, IHttpClientFactory httpClientFactory, Action<JobStatus> statusChanged)
    {
        var jobId = DateTimeOffset.Now.ToString("yyyyMMddHHmmssfff");
        var rootPath = Path.Combine(Path.GetFullPath(request.DownloadPath), "tasks", jobId);
        var layerJobs = new Dictionary<string, LayerTileJob>(StringComparer.OrdinalIgnoreCase);

        foreach (var layer in request.GetLayerDefinitions())
        {
            var layerRequest = request.ForSingleLayer(layer.Key) with { DownloadPath = rootPath };
            layerJobs[layer.Key] = LayerTileJob.Create(jobId, layerRequest, layer, httpClientFactory.CreateClient("tiles"));
        }

        var name = string.IsNullOrWhiteSpace(request.Name) ? "天地图瓦片下载任务" : request.Name.Trim();
        return new MultiLayerTileJob(jobId, name, request with { Name = name, DownloadPath = rootPath }, rootPath, layerJobs, statusChanged);
    }

    public static MultiLayerTileJob RestoreForResume(JobStatus status, IHttpClientFactory httpClientFactory, Action<JobStatus> statusChanged)
    {
        var layerJobs = new Dictionary<string, LayerTileJob>(StringComparer.OrdinalIgnoreCase);
        foreach (var layer in status.Request.GetLayerDefinitions())
        {
            var layerStatus = status.Layers.FirstOrDefault(item => item.Key.Equals(layer.Key, StringComparison.OrdinalIgnoreCase));
            var layerRequest = status.Request.ForSingleLayer(layer.Key) with { DownloadPath = status.OutputPath };
            layerJobs[layer.Key] = LayerTileJob.RestoreForResume(status.JobId, layerRequest, layer, httpClientFactory.CreateClient("tiles"), layerStatus);
        }

        return new MultiLayerTileJob(status.JobId, status.Name, status.Request with { DownloadPath = status.OutputPath }, status.OutputPath, layerJobs, statusChanged, status.CreatedAt);
    }

    public void Start()
    {
        Directory.CreateDirectory(OutputPath);
        Directory.CreateDirectory(ArtifactPath);
        foreach (var layerJob in layerJobs.Values)
        {
            layerJob.Start(() => statusChanged(GetStatus()));
        }
    }

    public void StopAll()
    {
        foreach (var layerJob in layerJobs.Values)
        {
            layerJob.Stop();
        }
    }

    public void ResumeAll()
    {
        foreach (var layerJob in layerJobs.Values)
        {
            layerJob.Resume(() => statusChanged(GetStatus()));
        }
    }

    public void StopLayer(string layer)
    {
        if (layerJobs.TryGetValue(layer, out var layerJob))
        {
            layerJob.Stop();
        }
    }

    public void ResumeLayer(string layer)
    {
        if (layerJobs.TryGetValue(layer, out var layerJob))
        {
            layerJob.Resume(() => statusChanged(GetStatus()));
        }
    }

    public void RetryLayer(string layer)
    {
        if (layerJobs.TryGetValue(layer, out var layerJob))
        {
            layerJob.RetryFailed(() => statusChanged(GetStatus()));
        }
    }

    public string? GetFailureRecordPath(string layer)
    {
        return layerJobs.TryGetValue(layer, out var layerJob) ? layerJob.FailureRecordPath : null;
    }

    public JobStatus GetStatus()
    {
        var layers = layerJobs.Values.Select(job => job.GetStatus()).OrderBy(status => status.Order).ToList();
        var total = layers.Sum(layer => layer.Total);
        var processed = layers.Sum(layer => layer.Processed);
        var completed = layers.Sum(layer => layer.Completed);
        var failed = layers.Sum(layer => layer.Failed);
        var state = ResolveState(layers);
        var completedAt = IsTerminal(state) ? DateTimeOffset.Now : (DateTimeOffset?)null;
        return new JobStatus(JobId, Name, state, total, processed, completed, failed, OutputPath, ArtifactPath, CreatedAt, completedAt, Request, layers, null);
    }

    private static string ResolveState(IReadOnlyCollection<LayerJobStatus> layers)
    {
        if (layers.Any(layer => layer.State is nameof(JobState.Running) or nameof(JobState.Pending))) return nameof(JobState.Running);
        if (layers.Any(layer => layer.State == nameof(JobState.Failed))) return nameof(JobState.Failed);
        if (layers.Any(layer => layer.State == nameof(JobState.CompletedWithFailures))) return nameof(JobState.CompletedWithFailures);
        if (layers.Any(layer => layer.State == nameof(JobState.Stopped))) return nameof(JobState.Stopped);
        return nameof(JobState.Completed);
    }

    private static bool IsTerminal(string state) => state is nameof(JobState.Completed) or nameof(JobState.CompletedWithFailures) or nameof(JobState.Stopped) or nameof(JobState.Failed);
}

public sealed class LayerTileJob
{
    private readonly HttpClient httpClient;
    private readonly DownloadRequest request;
    private readonly TileLayerDefinition layer;
    private readonly object gate = new();
    private ConcurrentQueue<TileDownloadItem> queue;
    private readonly ConcurrentBag<TileDownloadItem> failedItems = new();
    private CancellationTokenSource cancellation = new();
    private int completed;
    private int failed;
    private int processed;
    private string currentTile = "-";
    private JobState state = JobState.Pending;

    private LayerTileJob(string jobId, DownloadRequest request, TileLayerDefinition layer, HttpClient httpClient, List<TileDownloadItem> items)
    {
        JobId = jobId;
        this.request = request;
        this.layer = layer;
        this.httpClient = httpClient;
        queue = new ConcurrentQueue<TileDownloadItem>(items);
        Total = items.Count;
        OutputPath = Path.Combine(Path.GetFullPath(request.DownloadPath), layer.Key);
        FailureRecordPath = Path.Combine(OutputPath, "RecorderList.json");
    }

    public string JobId { get; }
    public int Total { get; private set; }
    public string OutputPath { get; }
    public string FailureRecordPath { get; }
    public bool IsRunning => state == JobState.Running;

    public static LayerTileJob Create(string jobId, DownloadRequest request, TileLayerDefinition layer, HttpClient httpClient)
    {
        var items = CreateItems(request, layer);

        return new LayerTileJob(jobId, request, layer, httpClient, items);
    }

    public static LayerTileJob RestoreForResume(string jobId, DownloadRequest request, TileLayerDefinition layer, HttpClient httpClient, LayerJobStatus? status)
    {
        var allItems = CreateItems(request, layer);
        var existing = allItems.Count(item => File.Exists(TilePath(Path.Combine(Path.GetFullPath(request.DownloadPath), layer.Key), item)));
        var remaining = allItems.Where(item => !File.Exists(TilePath(Path.Combine(Path.GetFullPath(request.DownloadPath), layer.Key), item))).ToList();
        var job = new LayerTileJob(jobId, request, layer, httpClient, remaining)
        {
            Total = allItems.Count,
            completed = existing,
            processed = existing,
            failed = 0,
            state = remaining.Count == 0 ? JobState.Completed : status?.State == nameof(JobState.Stopped) ? JobState.Stopped : JobState.Completed
        };
        return job;
    }

    private static List<TileDownloadItem> CreateItems(DownloadRequest request, TileLayerDefinition layer)
    {
        var tileIds = TileMath.CalculateTiles(request);
        return tileIds.Select((tile, index) =>
        {
            var server = index % 8;
            var url = $"http://t{server}.tianditu.gov.cn/{layer.Key}_w/wmts?SERVICE=WMTS&REQUEST=GetTile&VERSION=1.0.0&LAYER={layer.Key}&STYLE=default&TILEMATRIXSET=w&FORMAT=tiles&TILEMATRIX={tile.Z}&TILEROW={tile.Y}&TILECOL={tile.X}&tk={request.Token}";
            return new TileDownloadItem(index, tile.Z, tile.X, tile.Y, url);
        }).ToList();
    }

    public void Start(Action onChanged)
    {
        lock (gate)
        {
            if (IsRunning)
            {
                return;
            }

            cancellation = new CancellationTokenSource();
            state = JobState.Pending;
            _ = RunAsync(cancellation.Token, onChanged);
        }
    }

    public void Stop()
    {
        cancellation.Cancel();
    }

    public void Resume(Action onChanged)
    {
        lock (gate)
        {
            if (IsRunning || state != JobState.Stopped || queue.IsEmpty)
            {
                return;
            }

            cancellation = new CancellationTokenSource();
            state = JobState.Pending;
            _ = RunAsync(cancellation.Token, onChanged);
        }
    }

    public void RetryFailed(Action onChanged)
    {
        lock (gate)
        {
            if (IsRunning)
            {
                return;
            }

            var items = failedItems.ToArray().OrderBy(item => item.Index).ToList();
            if (items.Count == 0)
            {
                return;
            }

            while (failedItems.TryTake(out _))
            {
            }

            queue = new ConcurrentQueue<TileDownloadItem>(items);
            Total = items.Count;
            completed = 0;
            failed = 0;
            processed = 0;
            currentTile = "-";
            cancellation = new CancellationTokenSource();
            state = JobState.Pending;
            _ = RunAsync(cancellation.Token, onChanged);
        }
    }

    private async Task RunAsync(CancellationToken token, Action onChanged)
    {
        Directory.CreateDirectory(OutputPath);
        state = JobState.Running;
        onChanged();

        try
        {
            await DownloadQueueAsync(token, onChanged);
            state = token.IsCancellationRequested ? JobState.Stopped : failedItems.IsEmpty ? JobState.Completed : JobState.CompletedWithFailures;
        }
        catch
        {
            state = JobState.Failed;
        }
        finally
        {
            WriteFailureRecord();
            onChanged();
        }
    }

    private async Task DownloadQueueAsync(CancellationToken token, Action onChanged)
    {
        while (!token.IsCancellationRequested && queue.TryDequeue(out var item))
        {
            currentTile = $"{item.Z}/{item.X}/{item.Y}";
            var ok = await DownloadOneAsync(item, token);
            if (token.IsCancellationRequested)
            {
                RequeueCurrent(item);
                break;
            }

            Interlocked.Increment(ref processed);
            if (ok)
            {
                Interlocked.Increment(ref completed);
            }
            else
            {
                Interlocked.Increment(ref failed);
                failedItems.Add(item);
            }

            if (processed % 10 == 0 || processed == Total)
            {
                onChanged();
            }
        }
    }

    private void RequeueCurrent(TileDownloadItem item)
    {
        var remaining = queue.ToArray().OrderBy(tile => tile.Index).ToList();
        remaining.Insert(0, item);
        queue = new ConcurrentQueue<TileDownloadItem>(remaining);
        currentTile = "-";
    }

    private async Task<bool> DownloadOneAsync(TileDownloadItem item, CancellationToken token)
    {
        try
        {
            var directory = Path.Combine(OutputPath, item.Z.ToString(), item.X.ToString());
            Directory.CreateDirectory(directory);
            var savePath = TilePath(OutputPath, item);
            using var response = await httpClient.GetAsync(item.Url, token);
            if (!response.IsSuccessStatusCode || response.Content.Headers.ContentType?.MediaType?.StartsWith("text/", StringComparison.OrdinalIgnoreCase) == true)
            {
                return false;
            }

            await using var source = await response.Content.ReadAsStreamAsync(token);
            await using var target = File.Create(savePath);
            await source.CopyToAsync(target, token);
            return true;
        }
        catch
        {
            return false;
        }
    }

    private static string TilePath(string outputPath, TileDownloadItem item) => Path.Combine(outputPath, item.Z.ToString(), item.X.ToString(), $"{item.Y}.png");

    private void WriteFailureRecord()
    {
        var failures = failedItems
            .OrderBy(item => item.Index)
            .Select(item => new DownLoadRecorder(item.Url, item.Y.ToString(), item.Z.ToString(), item.X.ToString(), item.Index, OutputPath))
            .ToList();

        if (failures.Count == 0)
        {
            if (File.Exists(FailureRecordPath))
            {
                File.Delete(FailureRecordPath);
            }

            return;
        }

        Directory.CreateDirectory(OutputPath);
        File.WriteAllText(FailureRecordPath, JsonSerializer.Serialize(failures, JsonOptions.Default));
    }

    public LayerJobStatus GetStatus()
    {
        return new LayerJobStatus(
            layer.Key,
            layer.Name,
            layer.Order,
            state.ToString(),
            Total,
            processed,
            completed,
            failed,
            currentTile,
            OutputPath,
            FailureRecordPath,
            failedItems.Count,
            null);
    }
}

public static class TileMath
{
    public static List<TileId> CalculateTiles(DownloadRequest request)
    {
        var tileIds = new List<TileId>();
        for (var layer = 0; layer < request.MaxLayer; layer++)
        {
            var leftUp = GetTileId(request.LeftUpLon, request.LeftUpLat, layer);
            var rightDown = GetTileId(request.RightDownLon, request.RightDownLat, layer);
            for (var x = leftUp.X; x <= rightDown.X; x++)
            {
                for (var y = leftUp.Y; y <= rightDown.Y; y++)
                {
                    tileIds.Add(new TileId(layer, x, y));
                }
            }
        }

        return tileIds;
    }

    private static TileId GetTileId(double lon, double lat, int layer)
    {
        var x = (int)(Math.Pow(2.0, layer - 1) * (lon / 180.0 + 1.0));
        var y = (int)(Math.Pow(2.0, layer - 1) * (1.0 - Math.Log(Math.Tan(Math.PI * lat / 180.0) + Sec(Math.PI * lat / 180.0)) / Math.PI));
        return new TileId(layer, x, y);
    }

    private static double Sec(double x) => 1.0 / Math.Cos(x);
}

public static class TarGz
{
    public static void CreateFromDirectory(string sourceDirectory, string archivePath, string rootName, IReadOnlyCollection<string>? excludeDirectoryNames = null)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(archivePath)!);
        if (File.Exists(archivePath))
        {
            File.Delete(archivePath);
        }

        using var file = File.Create(archivePath);
        using var gzip = new GZipStream(file, CompressionLevel.Fastest);
        foreach (var path in Directory.EnumerateFiles(sourceDirectory, "*", SearchOption.AllDirectories))
        {
            var relative = Path.GetRelativePath(sourceDirectory, path).Replace('\\', '/');
            if (excludeDirectoryNames?.Any(name => relative.Equals(name, StringComparison.OrdinalIgnoreCase) || relative.StartsWith($"{name}/", StringComparison.OrdinalIgnoreCase)) == true)
            {
                continue;
            }

            WriteFile(gzip, path, $"{rootName}/{relative}");
        }

        gzip.Write(new byte[1024]);
    }

    private static void WriteFile(Stream target, string path, string entryName)
    {
        var info = new FileInfo(path);
        var header = new byte[512];
        WriteString(header, 0, 100, entryName);
        WriteOctal(header, 100, 8, 420);
        WriteOctal(header, 108, 8, 0);
        WriteOctal(header, 116, 8, 0);
        WriteOctal(header, 124, 12, info.Length);
        WriteOctal(header, 136, 12, new DateTimeOffset(info.LastWriteTimeUtc).ToUnixTimeSeconds());
        for (var i = 148; i < 156; i++) header[i] = 32;
        header[156] = (byte)'0';
        WriteString(header, 257, 6, "ustar");
        WriteString(header, 263, 2, "00");
        var checksum = header.Sum(item => (int)item);
        WriteOctal(header, 148, 8, checksum);
        target.Write(header);
        using var source = File.OpenRead(path);
        source.CopyTo(target);
        var padding = 512 - (info.Length % 512);
        if (padding is > 0 and < 512)
        {
            target.Write(new byte[padding]);
        }
    }

    private static void WriteString(byte[] buffer, int offset, int length, string value)
    {
        var bytes = Encoding.UTF8.GetBytes(value);
        Array.Copy(bytes, 0, buffer, offset, Math.Min(bytes.Length, length));
    }

    private static void WriteOctal(byte[] buffer, int offset, int length, long value)
    {
        var text = Convert.ToString(value, 8).PadLeft(length - 1, '0');
        var bytes = Encoding.ASCII.GetBytes(text);
        Array.Copy(bytes, 0, buffer, offset, Math.Min(bytes.Length, length - 1));
        buffer[offset + length - 1] = 0;
    }
}

public sealed record DownloadRequest
{
    public string Name { get; init; } = "天地图瓦片下载任务";
    public string Token { get; init; } = "";
    public string[] Layers { get; init; } = LayerCatalog.DefaultKeys;
    public double LeftUpLon { get; init; }
    public double LeftUpLat { get; init; }
    public double RightDownLon { get; init; }
    public double RightDownLat { get; init; }
    public int MaxLayer { get; init; }
    public string DownloadPath { get; init; } = "/data/tiles";

    public string? Validate()
    {
        if (Name.Length > 80) return "任务名称不能超过 80 个字符。";
        if (string.IsNullOrWhiteSpace(Token)) return "Token 不能为空。";
        if (Layers.Length == 0) return "至少选择一个图层。";
        if (GetLayerDefinitions().Count != Layers.Distinct(StringComparer.OrdinalIgnoreCase).Count()) return "存在不支持的图层类型。";
        if (MaxLayer < 1 || MaxLayer > 18) return "最大层级必须在 1 到 18 之间。";
        if (LeftUpLon < -180 || LeftUpLon > 180 || RightDownLon < -180 || RightDownLon > 180) return "经度必须在 -180 到 180 之间。";
        if (LeftUpLat < -85 || LeftUpLat > 85 || RightDownLat < -85 || RightDownLat > 85) return "纬度必须在 -85 到 85 之间。";
        if (LeftUpLon > RightDownLon) return "左上经度不能大于右下经度。";
        if (LeftUpLat < RightDownLat) return "左上纬度不能小于右下纬度。";
        return null;
    }

    public List<TileLayerDefinition> GetLayerDefinitions()
    {
        return Layers
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .Select(layer => LayerCatalog.TryGet(layer, out var definition) ? definition : null)
            .Where(definition => definition is not null)
            .Cast<TileLayerDefinition>()
            .OrderBy(definition => definition.Order)
            .ToList();
    }

    public DownloadRequest ForSingleLayer(string layer)
    {
        return this with { Layers = new[] { layer } };
    }
}

public static class LayerCatalog
{
    private static readonly Dictionary<string, TileLayerDefinition> Layers = new(StringComparer.OrdinalIgnoreCase)
    {
        ["img"] = new("img", "卫星图", 1),
        ["cia"] = new("cia", "路网", 2),
        ["vec"] = new("vec", "电子图", 3)
    };

    public static readonly string[] DefaultKeys = { "img", "cia", "vec" };

    public static bool TryGet(string key, out TileLayerDefinition definition) => Layers.TryGetValue(key, out definition!);
}

public static class Initials
{
    public const string Task = "tdt";
    public static string ForLayer(string layer) => layer.ToLowerInvariant() switch
    {
        "img" => "wxt",
        "cia" => "lw",
        "vec" => "dzt",
        _ => layer.ToLowerInvariant()
    };
}

public readonly record struct ArchiveInfo(string Path, string FileName);
public sealed record TileLayerDefinition(string Key, string Name, int Order);
public sealed record TileId(int Z, int X, int Y);
public sealed record TileDownloadItem(int Index, int Z, int X, int Y, string Url);
public sealed record DownLoadRecorder(string Url, string Name, string Layer, string X, int Index, string Path);
public sealed record ArtifactInfo(string FileName, string Path, DateTimeOffset CreatedAt);
public sealed record JobStatus(string JobId, string Name, string State, int Total, int Processed, int Completed, int Failed, string OutputPath, string ArtifactPath, DateTimeOffset CreatedAt, DateTimeOffset? CompletedAt, DownloadRequest Request, List<LayerJobStatus> Layers, ArtifactInfo? Artifact);
public sealed record LayerJobStatus(string Key, string Name, int Order, string State, int Total, int Processed, int Completed, int Failed, string CurrentTile, string OutputPath, string FailureRecordPath, int FailureQueueCount, ArtifactInfo? Artifact);
public enum JobState { Pending, Running, Completed, CompletedWithFailures, Stopped, Failed }

public static class JsonOptions
{
    public static readonly JsonSerializerOptions Default = new()
    {
        WriteIndented = true,
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull
    };
}
