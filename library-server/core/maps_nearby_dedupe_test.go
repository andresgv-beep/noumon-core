package main

import "testing"

// Un mismo nombre aparece varias veces en el mapa: la plaza "Creu Gran" y sus
// paradas de bus son puntos propios de OSM, todos a menos de 200 m. Es dato
// correcto, pero llenaba media lista de lugares cercanos con la misma palabra.
func TestDedupeNearbyByNameKeepsNearest(t *testing.T) {
	// Ya ordenada por distancia, que es como la recibe la funcion.
	hits := []nearbyHit{
		{Name: "Creu Gran", CategoryCode: "transport", Distance: 50},
		{Name: "Sant Pere", CategoryCode: "culture", Distance: 158},
		{Name: "Creu Gran", CategoryCode: "culture", Distance: 158},
		{Name: "creu gran", CategoryCode: "transport", Distance: 162},
		{Name: "Creu Gran", CategoryCode: "transport", Distance: 178},
	}
	out := dedupeNearbyByName(hits)

	if len(out) != 3 {
		t.Fatalf("esperaba 3 sitios distintos, salieron %d: %+v", len(out), out)
	}
	if out[0].Name != "Creu Gran" || out[0].Distance != 50 {
		t.Fatalf("de las repetidas debe quedar la mas cercana, quedo %+v", out[0])
	}
	// Mismo nombre en otra categoria SI es otro sitio: la plaza y la parada se
	// distinguen y las dos siguen en la lista.
	var culture bool
	for _, hit := range out {
		if hit.Name == "Creu Gran" && hit.CategoryCode == "culture" {
			culture = true
		}
	}
	if !culture {
		t.Fatalf("el mismo nombre en otra categoria no debe desaparecer: %+v", out)
	}
}

// Las mayusculas y los acentos del nombre no deben crear un sitio nuevo.
func TestDedupeNearbyByNameIgnoresCase(t *testing.T) {
	out := dedupeNearbyByName([]nearbyHit{
		{Name: "Farmàcia Torres", CategoryCode: "health", Distance: 23},
		{Name: "FARMÀCIA TORRES", CategoryCode: "health", Distance: 240},
	})
	if len(out) != 1 || out[0].Distance != 23 {
		t.Fatalf("esperaba una sola farmacia, la mas cercana: %+v", out)
	}
}
