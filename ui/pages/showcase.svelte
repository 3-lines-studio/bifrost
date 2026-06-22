<script lang="ts">
  import "../style.css";
  import {
    Button,
    Badge,
    Card,
    CardHeader,
    CardTitle,
    CardDescription,
    CardContent,
    CardFooter,
    Alert,
    AlertTitle,
    AlertDescription,
    Separator,
    Skeleton,
    Spinner,
    Label,
    Kbd,
    Avatar,
    AvatarImage,
    AvatarFallback,
    Input,
    Textarea,
  } from "../src";

  let dark = $state(false);
  function toggleDark() {
    dark = !dark;
    document.documentElement.classList.toggle("dark", dark);
  }
</script>

<svelte:head>
  <title>bifrost-ui — Component Showcase</title>
</svelte:head>

<div class="min-h-screen bg-background text-foreground">
  <header class="border-b">
    <div class="container mx-auto flex items-center justify-between px-6 py-4">
      <h1 class="text-xl font-bold">bifrost-ui</h1>
      <Button variant="outline" size="sm" onclick={toggleDark}>
        {dark ? "Light" : "Dark"} mode
      </Button>
    </div>
  </header>

  <main class="container mx-auto max-w-4xl space-y-8 px-6 py-8">

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Button</h2>
      <div class="flex flex-wrap items-center gap-4 rounded-lg border bg-card p-6">
        <Button variant="default">Default</Button>
        <Button variant="secondary">Secondary</Button>
        <Button variant="destructive">Destructive</Button>
        <Button variant="outline">Outline</Button>
        <Button variant="ghost">Ghost</Button>
        <Button variant="link">Link</Button>
      </div>
      <div class="flex flex-wrap items-center gap-4 rounded-lg border bg-card p-6">
        <Button size="sm">Small</Button>
        <Button size="default">Default</Button>
        <Button size="lg">Large</Button>
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Badge</h2>
      <div class="flex flex-wrap items-center gap-4 rounded-lg border bg-card p-6">
        <Badge variant="default">Default</Badge>
        <Badge variant="secondary">Secondary</Badge>
        <Badge variant="destructive">Destructive</Badge>
        <Badge variant="error">Error</Badge>
        <Badge variant="outline">Outline</Badge>
        <Badge variant="info">Info</Badge>
        <Badge variant="success">Success</Badge>
        <Badge variant="warning">Warning</Badge>
      </div>
      <div class="flex flex-wrap items-center gap-4 rounded-lg border bg-card p-6">
        <Badge size="sm">Small</Badge>
        <Badge size="default">Default</Badge>
        <Badge size="lg">Large</Badge>
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Card</h2>
      <div class="rounded-lg border bg-card p-6">
        <Card>
          <CardHeader>
            <CardTitle>Card Title</CardTitle>
            <CardDescription>Card description with some supporting text below the title.</CardDescription>
          </CardHeader>
          <CardContent>
            <p>This is the main content area of the card. Cards are used to group related information and actions.</p>
          </CardContent>
          <CardFooter>
            <Button variant="outline" size="sm">Cancel</Button>
            <Button size="sm">Save</Button>
          </CardFooter>
        </Card>
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Alert</h2>
      <div class="flex flex-col gap-4 rounded-lg border bg-card p-6">
        <Alert>
          <AlertTitle>Information</AlertTitle>
          <AlertDescription>This is a default alert with a title and description.</AlertDescription>
        </Alert>
        <Alert variant="destructive">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>Something went wrong. Please try again.</AlertDescription>
        </Alert>
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Separator</h2>
      <div class="space-y-4 rounded-lg border bg-card p-6">
        <div class="space-y-4">
          <p class="text-sm text-muted-foreground">Above the separator</p>
          <Separator />
          <p class="text-sm text-muted-foreground">Below the separator</p>
        </div>
        <div class="flex h-8 items-center gap-4">
          <span class="text-sm text-muted-foreground">Left</span>
          <Separator orientation="vertical" />
          <span class="text-sm text-muted-foreground">Right</span>
        </div>
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Skeleton</h2>
      <div class="flex flex-wrap items-center gap-6 rounded-lg border bg-card p-6">
        <Skeleton class="h-20 w-full max-w-sm rounded-lg" />
        <Skeleton class="size-16 rounded-full" />
        <div class="space-y-2">
          <Skeleton class="h-4 w-60" />
          <Skeleton class="h-4 w-48" />
          <Skeleton class="h-4 w-36" />
        </div>
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Spinner</h2>
      <div class="flex flex-wrap items-center gap-6 rounded-lg border bg-card p-6">
        <Spinner size="sm" />
        <Spinner size="default" />
        <Spinner size="lg" />
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Label</h2>
      <div class="flex flex-wrap items-end gap-6 rounded-lg border bg-card p-6">
        <div class="grid gap-2">
          <Label for="name">Name</Label>
          <Input id="name" placeholder="Enter your name" />
        </div>
        <div class="grid gap-2">
          <Label for="email">Email</Label>
          <Input id="email" type="email" placeholder="you@example.com" />
        </div>
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Kbd</h2>
      <div class="flex flex-wrap items-center gap-4 rounded-lg border bg-card p-6">
        <Kbd>⌘</Kbd><span class="text-sm text-muted-foreground">+</span><Kbd>K</Kbd>
        <Kbd>Shift</Kbd><span class="text-sm text-muted-foreground">+</span><Kbd>Tab</Kbd>
        <Kbd>Ctrl</Kbd><span class="text-sm text-muted-foreground">+</span><Kbd>C</Kbd>
        <Kbd>Enter</Kbd>
        <Kbd>Esc</Kbd>
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Avatar</h2>
      <div class="flex flex-wrap items-center gap-4 rounded-lg border bg-card p-6">
        <Avatar>
          <AvatarImage src="https://github.com/svelte.png" alt="Svelte" />
          <AvatarFallback>SV</AvatarFallback>
        </Avatar>
        <Avatar>
          <AvatarFallback>JD</AvatarFallback>
        </Avatar>
        <Avatar>
          <AvatarFallback>AB</AvatarFallback>
        </Avatar>
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Input</h2>
      <div class="flex flex-col gap-4 rounded-lg border bg-card p-6">
        <Input placeholder="Default input" />
        <Input placeholder="Disabled input" disabled />
        <Input placeholder="With placeholder text" value="Prefilled value" />
      </div>
    </section>

    <section class="space-y-3">
      <h2 class="text-lg font-semibold">Textarea</h2>
      <div class="rounded-lg border bg-card p-6">
        <Textarea placeholder="Type your message here..." />
      </div>
    </section>

  </main>
</div>
