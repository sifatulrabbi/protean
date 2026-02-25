export default function ThreadLoading() {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Messages skeleton */}
      <div className="min-h-0 flex-1 overflow-hidden px-4 py-6">
        <div className="flex flex-col gap-6">
          {/* Assistant message */}
          <div className="flex flex-col gap-2">
            <div className="h-4 w-16 animate-pulse rounded bg-muted" />
            <div className="flex flex-col gap-1.5 rounded-lg">
              <div className="h-3.5 w-3/4 animate-pulse rounded bg-muted" />
              <div className="h-3.5 w-full animate-pulse rounded bg-muted" />
              <div className="h-3.5 w-2/3 animate-pulse rounded bg-muted" />
            </div>
          </div>

          {/* User message */}
          <div className="flex flex-col items-end gap-2">
            <div className="h-4 w-12 animate-pulse rounded bg-muted" />
            <div className="flex w-2/3 flex-col gap-1.5 rounded-lg">
              <div className="h-3.5 w-full animate-pulse rounded bg-muted" />
              <div className="h-3.5 w-4/5 animate-pulse rounded bg-muted" />
            </div>
          </div>

          {/* Assistant message */}
          <div className="flex flex-col gap-2">
            <div className="h-4 w-16 animate-pulse rounded bg-muted" />
            <div className="flex flex-col gap-1.5">
              <div className="h-3.5 w-full animate-pulse rounded bg-muted" />
              <div className="h-3.5 w-5/6 animate-pulse rounded bg-muted" />
              <div className="h-3.5 w-3/4 animate-pulse rounded bg-muted" />
              <div className="h-3.5 w-full animate-pulse rounded bg-muted" />
              <div className="h-3.5 w-1/2 animate-pulse rounded bg-muted" />
            </div>
          </div>

          {/* User message */}
          <div className="flex flex-col items-end gap-2">
            <div className="h-4 w-12 animate-pulse rounded bg-muted" />
            <div className="flex w-1/2 flex-col gap-1.5">
              <div className="h-3.5 w-full animate-pulse rounded bg-muted" />
            </div>
          </div>
        </div>
      </div>

      {/* Prompt input skeleton */}
      <div className="sticky bottom-0 bg-background pt-2 pb-4">
        <div className="rounded-xl border bg-card p-3">
          <div className="mb-3 h-12 animate-pulse rounded bg-muted" />
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="h-8 w-8 animate-pulse rounded-md bg-muted" />
              <div className="h-8 w-32 animate-pulse rounded-full bg-muted" />
            </div>
            <div className="h-8 w-8 animate-pulse rounded-full bg-muted" />
          </div>
        </div>
      </div>
    </div>
  );
}
