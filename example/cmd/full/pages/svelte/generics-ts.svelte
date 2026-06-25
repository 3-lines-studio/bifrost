<script lang="ts" generics="T extends Record<string, unknown>">
  interface Props {
    data: T;
  }

  let { data }: Props = $props();

  // Optional parameter with `?` — triggers Svelte 5.56.3's buggy
  // internal TS stripping (leaves stray `?` producing invalid JS).
  function summarize(jump?: boolean): string {
    return Object.keys(data).join(",") + (jump ? "!" : "");
  }

  let summary = $derived(summarize());
</script>

<p class="summary">{summary}</p>
