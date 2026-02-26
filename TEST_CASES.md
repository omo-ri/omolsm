# Manual Test Cases
# Run: go run cmd/invertedindex/main.go
# Make sure configs/invertedindex.yaml points source.dir to ./documents

# ===========================================================================
# 1. Single term queries
# ===========================================================================

# > scientist
# Expected: space.txt, climate.txt, ai.txt, mars.txt, ocean.txt
# Reason: all English docs mention "scientists" (stemmed to "scientist")

# > ocean
# Expected: climate.txt, ocean.txt
# Reason: only these two mention "ocean"

# > Mars
# Expected: space.txt, mars.txt
# Reason: space.txt mentions Mars exploration, mars.txt is about Mars

# > ракета
# Expected: cosmos.txt
# Reason: only Russian space doc mentions "ракета" (rocket)

# > nonexistent
# Expected: No documents found.

# ===========================================================================
# 2. AND queries
# ===========================================================================

# > ocean AND coral
# Expected: climate.txt, ocean.txt
# Reason: both mention ocean and coral reefs

# > astronaut AND Mars
# Expected: mars.txt, space.txt
# Reason: both discuss astronauts going to Mars

# > scientist AND AI
# Expected: ai.txt
# Reason: only ai.txt has both scientists and AI

# > rocket AND ocean
# Expected: No documents found.
# Reason: no doc has both topics

# ===========================================================================
# 3. OR queries
# ===========================================================================

# > dolphin OR whale
# Expected: ocean.txt
# Reason: only ocean.txt mentions dolphins and whales

# > Moon OR Mars
# Expected: space.txt, mars.txt
# Reason: space.txt mentions Moon, mars.txt mentions Mars,
#         space.txt also mentions Mars

# > ракета OR двигатель
# Expected: cosmos.txt
# Reason: Russian space doc has both words

# ===========================================================================
# 4. AND NOT queries
# ===========================================================================

# > scientist AND NOT ocean
# Expected: space.txt, ai.txt, mars.txt
# Reason: all have "scientists" except climate and ocean are excluded
#         (climate.txt has ocean, ocean.txt has ocean)

# > ocean AND NOT coral
# Expected: (empty or subset)
# Reason: both ocean docs mention coral, so result may be empty

# > future AND NOT AI
# Expected: space.txt, climate.txt, mars.txt, cosmos.txt (depending on stemming)
# Reason: several docs mention "future" but not AI-related terms

# ===========================================================================
# 5. Nested queries
# ===========================================================================

# > (ocean OR climate) AND scientist
# Expected: climate.txt, ocean.txt
# Reason: both match ocean/climate AND have scientists

# > (Mars OR Moon) AND astronaut AND NOT rover
# Expected: space.txt
# Reason: space.txt has Moon+astronaut, mars.txt has rover so excluded

# > (rocket OR engine) AND space AND NOT ocean
# Expected: space.txt, mars.txt
# Reason: both mention rocket/engine + space, neither is about ocean

# ===========================================================================
# 6. Russian queries
# ===========================================================================

# > учёные
# Expected: cosmos.txt, ai_ru.txt
# Reason: both Russian docs mention "учёные" (scientists)

# > космонавт AND ракета ??
# Expected: cosmos.txt
# Reason: only cosmos.txt has both

# ===========================================================================
# 7. Edge cases
# ===========================================================================

# > the
# Expected: No documents found.
# Reason: "the" is a stop word, removed during indexing

# > a
# Expected: No documents found.
# Reason: "a" is a stop word

# > AND
# Expected: Error: unexpected operator
# Reason: operator without operands

# > (ocean AND
# Expected: Error: unexpected end of query / missing parenthesis
