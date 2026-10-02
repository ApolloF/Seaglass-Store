<script lang="ts">
  // A game's picture: downloaded art when there is some, otherwise the
  // generated landscape. `kind` picks which picture (cover is portrait,
  // hero is wide, backdrop is 16:9 and sharp enough to fill the screen).
  import { artFor } from "../lib/art";
  import type { Game } from "../lib/types";

  let { game, kind = "cover" }: { game: Pick<Game, "key" | "meta">; kind?: "cover" | "hero" | "backdrop" } = $props();

  // What to try, in order: a backdrop falls back to the hero, and a hero to
  // the cover, which is better than nothing. A picture that fails to load
  // moves on to the next.
  const chain = $derived(
    (kind === "cover" ? [game.meta?.cover] : kind === "hero" ? [game.meta?.hero, game.meta?.cover] : [game.meta?.backdrop, game.meta?.hero, game.meta?.cover]).filter(
      (s): s is string => !!s,
    ),
  );
  let at = $state(0);
  // Start over only when the pictures themselves change, not on every
  // update of the game (playtime, favourite).
  const chainKey = $derived(chain.join("|"));
  $effect(() => {
    chainKey;
    at = 0;
  });
  const src = $derived(chain[at]);
  // A stand-in for a backdrop (a small banner or the cover) would look
  // pixelated stretched over the screen: it's blurred into a soft
  // background instead.
  const soft = $derived(kind === "backdrop" && !!src && src !== game.meta?.backdrop);
  const a = $derived(artFor(game.key));
</script>

{#if src}
  <img class="img" class:soft {src} alt="" loading="lazy" decoding="async" draggable="false" onerror={() => at++} />
{:else}
  <div class="art" style:background={a.sky} aria-hidden="true">
    {#if a.starOp}<div class="layer" style:background-image={a.stars} style:opacity={a.starOp}></div>{/if}
    <div class="sun" style:left={a.sunL} style:top={a.sunT} style:width={a.sunW} style:background={a.sun} style:mask-image={a.sunMask}></div>
    <div class="layer" style:background={a.far} style:clip-path={a.farClip}></div>
    <div class="layer" style:background={a.fog}></div>
    <div class="layer" style:background={a.mid} style:clip-path={a.midClip}></div>
    {#if a.floorH !== "0%"}<div class="floor" style:height={a.floorH} style:background={a.floor}></div>{/if}
    <div class="layer" style:background={a.near} style:clip-path={a.nearClip}></div>
  </div>
{/if}

<style>
  .img,
  .art {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
  }
  .img {
    object-fit: cover;
  }
  .img.soft {
    filter: blur(28px) saturate(1.25) brightness(0.85);
    transform: scale(1.15);
  }
  .art {
    overflow: hidden;
  }
  .layer {
    position: absolute;
    inset: 0;
  }
  .sun {
    position: absolute;
    aspect-ratio: 1;
    border-radius: 50%;
  }
  .floor {
    position: absolute;
    left: -50%;
    right: -50%;
    bottom: 0;
  }
</style>
