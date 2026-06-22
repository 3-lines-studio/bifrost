<script lang="ts">
  import { createModuleContext, getModuleContext, type ModuleStatus } from "./avatar-context.svelte.ts";

  createModuleContext();
  let localStatus = $state<ModuleStatus>("active");
</script>

<p class="local">{localStatus}</p>
<p class="context">{getModuleContext()?.status ?? "missing"}</p>
