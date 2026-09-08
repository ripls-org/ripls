import 'package:ripls/presentation/models/drill_down_data.dart';

/// Reference tables shared across all drill-down metric builders.
const moneySavedReferences = [
  ResearchReference(
    id: 1,
    citation: 'Library of Things Impact Methodology (2024)',
    supportLevel: 'Directional',
    usedFor: 'Prevented purchase rate (50% for loans)',
  ),
  ResearchReference(
    id: 2,
    citation: 'Benthyg Cymru Sharing Impact Survey',
    supportLevel: 'Directional',
    usedFor: 'Prevented purchase rate validation',
  ),
];

const emissionsReferences = [
  ResearchReference(
    id: 1,
    citation: 'Edinburgh Tool Library Carbon Methodology',
    supportLevel: 'Direct',
    usedFor: 'Material category emission factors',
  ),
  ResearchReference(
    id: 2,
    citation: 'University of Bath ICE Database',
    supportLevel: 'Direct',
    usedFor: 'Material embodied carbon factors (kg CO₂e/kg)',
  ),
  ResearchReference(
    id: 3,
    citation: 'Decarbon Open-Source EEIO Dataset (CC BY-SA 4.0)',
    supportLevel: 'Direct',
    usedFor: 'Spend-based emission factors (kg CO₂e/USD)',
  ),
  ResearchReference(
    id: 4,
    citation: 'EPA WARM Model',
    supportLevel: 'Direct',
    usedFor: 'Waste diversion carbon factor (1.0 kg CO₂e/kg)',
  ),
];

const timeSavedReferences = [
  ResearchReference(
    id: 1,
    citation: 'Consumer Shopping Time Research',
    supportLevel: 'Directional',
    usedFor: 'Default shopping/research time (120 min)',
  ),
];

const qualityTimeReferences = [
  ResearchReference(
    id: 1,
    citation: 'Kahneman et al. — Time Use and Wellbeing',
    supportLevel: 'Conceptual',
    usedFor: 'Duration as base unit for social contact quality',
  ),
  ResearchReference(
    id: 2,
    citation:
        'Cacioppo & Cacioppo (2014) — Social Relationships and Health',
    supportLevel: 'Directional',
    usedFor: 'Modality rank ordering (in-person > video > phone > text)',
  ),
  ResearchReference(
    id: 3,
    citation: 'Dunbar (1992, 2010) — Social Brain Hypothesis',
    supportLevel: 'Directional',
    usedFor: 'Group size and tie strength factors',
  ),
  ResearchReference(
    id: 4,
    citation: 'Baumeister & Leary (1995) — Need to Belong',
    supportLevel: 'Conceptual',
    usedFor: 'Reciprocity and vulnerability as social quality factors',
  ),
];
