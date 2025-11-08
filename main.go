package main

import (
	// "bufio" // Ya no se usa
	// "bytes" // Ya no se usa
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http" // ¡Importante!
	"net/url"  // ¡Importante!
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/ollama/ollama/api" // --- ¡LÍNEA CORREGIDA! ---

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/fatih/color"
	"github.com/peterh/liner"
)

// --- ¡NUEVO! Structs para la API de wttr.in ---
type WttrWeatherDesc struct {
	Value string `json:"value"`
}
type WttrCurrentCondition struct {
	Temp_C      string            `json:"temp_C"`
	FeelsLikeC  string            `json:"FeelsLikeC"`
	WeatherDesc []WttrWeatherDesc `json:"weatherDesc"`
}
type WttrResponse struct {
	CurrentCondition []WttrCurrentCondition `json:"current_condition"`
	NearestArea []struct {
		AreaName []struct {
			Value string `json:"value"`
		} `json:"areaName"`
		Country []struct {
			Value string `json:"value"`
		} `json:"country"`
	} `json:"nearest_area"`
}


// --- Variables Globales de Estilo (Sin cambios) ---
var (
	logoMap    map[string][]string
	colorMap   map[string][]string

	styleHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))

	cSystem   = color.New(color.FgYellow).SprintFunc()
	cError    = color.New(color.FgRed, color.Bold).SprintFunc()
	cPrompt   = color.New(color.FgHiCyan, color.Bold).SprintFunc()
	cIA       = color.New(color.FgGreen).SprintFunc()
	cModel    = color.New(color.FgMagenta).SprintFunc()
)

// clearScreen (Sin cambios)
func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

// loadLogos (Sin cambios)
func loadLogos() {
	file, err := os.Open("logos.json")
	if err != nil {
		fmt.Println(cError("Error: No se encontró logos.json. Saltando logos."))
		logoMap = make(map[string][]string)
		return
	}
	defer file.Close()
	bytes, err := io.ReadAll(file)
	if err != nil {
		log.Fatal(cError(fmt.Sprintf("Error fatal: No se pudo leer logos.json: %v", err)))
	}
	if err := json.Unmarshal(bytes, &logoMap); err != nil {
		log.Fatal(cError(fmt.Sprintf("Error fatal: No se pudo parsear logos.json: %v", err)))
	}
}

// createColorMap (Sin cambios)
func createColorMap() {
	colorMap = map[string][]string{
		"llama":    {"#B721FF", "#21D4FD"},
		"mistral":  {"#FF8008", "#FFC837"},
		"gemma":    {"#007BFF", "#00C6FF"},
		"phi3":     {"#6A11CB", "#2575FC"},
		"deepseek": {"#1D2B64", "#F8CDDA"},
		"qwen":     {"#4E49E7", "#A849E7"},
		"gpt":      {"#74AA9C", "#2CB77F"},
		"default":  {"#FFFFFF", "#EAEAEA"},
	}
}

// getLogoKey (Sin cambios)
func getLogoKey(modelName string) string {
	lowerName := strings.ToLower(modelName)
	for key := range logoMap {
		if strings.Contains(lowerName, key) {
			return key
		}
	}
	return "default"
}

// printLogo (Sin cambios)
func printLogo(modelName string) {
	logoKey := getLogoKey(modelName)
	logoLines, ok := logoMap[logoKey]
	if !ok || len(logoLines) == 0 {
		return
	}
	colors, ok := colorMap[logoKey]
	if !ok {
		colors = colorMap["default"]
	}
	startColor, _ := colorful.Hex(colors[0])
	endColor, _ := colorful.Hex(colors[1])
	numLines := len(logoLines)
	for i, line := range logoLines {
		var t float64
		if numLines <= 1 {
			t = 1.0
		} else {
			t = float64(i) / float64(numLines-1)
		}
		interpolatedColor := startColor.BlendHcl(endColor, t)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(interpolatedColor.Hex()))
		fmt.Println(style.Render(line))
	}
}

// --- printHeader (¡ACTUALIZADO!) ---
func printHeader() {
	fmt.Println()
	// --- ¡CAMBIO DE VERSIÓN AQUÍ! ---
	fmt.Println(styleHeader.Render("  Terminal Aumentada por IA (v17.2)"))
	fmt.Println(styleHeader.Render("  Creado por: Daniel Serrano Armenta <dani.eus79@gmail.com>"))
	fmt.Println(styleHeader.Render("  Copyright (c) 2025 Daniel Serrano Armenta. Ver LICENSE para más detalles."))
	fmt.Println(styleHeader.Render("  Github: https://github.com/danitxu79/terminal-ia"))
}

// --- printHelp (¡ACTUALIZADO!) ---
func printHelp() {
	fmt.Println()
	fmt.Println(cSystem("--- Ayuda: Comandos Disponibles ---"))
	fmt.Println(cPrompt("  /<petición> ") + cIA("- Pide un comando de shell (ej. /listar archivos .go)"))
	fmt.Println(cPrompt("  /chat <pregunta> ") + cIA("- Inicia una conversación de chat (ej. /chat ¿qué es Docker?)"))
	fmt.Println(cPrompt("  /tiempo <lugar>  ") + cIA("- Consulta el tiempo (sin API key) (ej. /tiempo Madrid)"))
	fmt.Println(cPrompt("  /model       ") + cIA("- Vuelve a mostrar el selector de modelos."))
	fmt.Println(cPrompt("  /ask         ") + cIA("- Desactiva el modo 'auto' y vuelve a pedir confirmación."))
	fmt.Println(cPrompt("  /help        ") + cIA("- Muestra este menú de ayuda."))
	fmt.Println(cPrompt("  cd <dir>     ") + cIA("- Cambia el directorio actual (comando interno)."))
	fmt.Println(cPrompt("  exit / quit  ") + cIA("- Cierra la terminal de IA (también Ctrl+D)."))
	fmt.Println(cSystem("------------------------------------"))
	fmt.Println()
}

// warmUpModel (Sin cambios)
func warmUpModel(client *api.Client, modelName string) {
	ctx := context.Background()
	req := &api.GenerateRequest{
		Model:  modelName,
		Prompt: "hola",
		Stream: new(bool),
	}
	responseHandler := func(r api.GenerateResponse) error { return nil }
	if err := client.Generate(ctx, req, responseHandler); err != nil {
		fmt.Fprintln(os.Stderr, cError(fmt.Sprintf("Advertencia: Fallo al 'calentar' el modelo: %v", err)))
	}
}

// chooseModel (Sin cambios)
func chooseModel(client *api.Client, state *liner.State) string {
	fmt.Println(cSystem("Consultando modelos de Ollama disponibles..."))

	ctx := context.Background()
	resp, err := client.List(ctx)
	if err != nil {
		log.Fatal(cError(fmt.Sprintf("Error fatal: No se pudo listar los modelos de Ollama: %v", err)))
	}
	if len(resp.Models) == 0 {
		log.Fatal(cError("Error fatal: No tienes ningún modelo de Ollama descargado. (Usa 'ollama pull ...')"))
	}

	fmt.Println(cSystem("--- Elige un modelo de IA ---"))
	fmt.Println(cSystem("------------------------------"))
	for i, model := range resp.Models {
		fmt.Printf("%d: %s\n", i+1, model.Name)
	}
	fmt.Println(cSystem("------------------------------"))

	var choice int
	for {
		prompt := "Introduce el número del modelo: "
		input, err := state.Prompt(prompt)
		if err != nil {
			if err == io.EOF || err == liner.ErrPromptAborted {
				log.Fatal(cError("\nSelección cancelada. Saliendo."))
			}
			log.Fatal(cError(fmt.Sprintf("Error al leer la selección: %v", err)))
		}

		choice, err = strconv.Atoi(strings.TrimSpace(input))
		if err != nil || choice < 1 || choice > len(resp.Models) {
			fmt.Println(cError("Selección inválida. Introduce un número de la lista."))
		} else {
			state.AppendHistory(input)
			break
		}
	}
	return resp.Models[choice-1].Name
}

// --- main (¡ACTUALIZADO!) ---
func main() {
	loadLogos()
	createColorMap()
	clearScreen()

	client, err := api.ClientFromEnvironment()
	if err != nil {
		log.Fatal(cError(fmt.Sprintf("Error fatal: No se pudo crear el cliente de Ollama: %v", err)))
	}

	state := liner.NewLiner()
	defer state.Close()
	state.SetCtrlCAborts(true)

	selectedModel := chooseModel(client, state)

	clearScreen()
	msg := fmt.Sprintf("Cargando modelo \"%s\" en memoria...\n(Esto puede tardar unos segundos)", selectedModel)
	fmt.Println(cSystem(msg))
	warmUpModel(client, selectedModel)

	clearScreen()
	printLogo(selectedModel)
	printHeader()
	fmt.Println(cSystem("\n  Consejo: Escribe /help para ver todos los comandos."))

	var alwaysExecute bool = false
	var isFirstLoop bool = true

	for {
		if isFirstLoop {
			isFirstLoop = false
			fmt.Println()
		} else {
			fmt.Println(cSystem("──────────────────────────────────────────────────"))
		}

		var prompt string
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "(error dir)"
		} else {
			home, err := os.UserHomeDir()
			if err == nil && strings.HasPrefix(cwd, home) {
				cwd = "~" + strings.TrimPrefix(cwd, home)
			}
		}

		var promptPrefix string
		if alwaysExecute {
			promptPrefix = "ia (auto) "
		} else {
			promptPrefix = "ia "
		}
		prompt = fmt.Sprintf("%s[%s]> %s >>> ", promptPrefix, selectedModel, cwd)

		input, err := state.Prompt(prompt)
		if err != nil {
			if err == io.EOF {
				break
			} else if err == liner.ErrPromptAborted {
				fmt.Println(cSystem("\nPrompt cancelado. Escribe 'exit' o Ctrl+D para salir."))
				continue
			} else {
				fmt.Println(cError(fmt.Sprintf("Error al leer la entrada: %v", err)))
				continue
			}
		}

		if input != "" {
			state.AppendHistory(input)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			break
		}

		if strings.HasPrefix(input, "cd ") {
			dir := strings.TrimSpace(strings.TrimPrefix(input, "cd "))
			if dir == "" || dir == "~" {
				home, err := os.UserHomeDir()
				if err != nil {
					fmt.Fprintln(os.Stderr, cError(fmt.Sprintf("Error al encontrar el home dir: %v", err)))
					continue
				}
				dir = home
			}
			if err := os.Chdir(dir); err != nil {
				fmt.Fprintln(os.Stderr, cError(err.Error()))
			}
			continue

		} else if input == "/model" {
			selectedModel = chooseModel(client, state)
			clearScreen()
			msg := fmt.Sprintf("Cargando modelo \"%s\" en memoria...\n(Esto puede tardar unos segundos)", selectedModel)
			fmt.Println(cSystem(msg))
			warmUpModel(client, selectedModel)
			clearScreen()
			printLogo(selectedModel)
			printHeader()
			fmt.Println(cSystem("\n  Consejo: Escribe /help para ver todos los comandos."))
			isFirstLoop = true
			continue

		} else if input == "/ask" {
			alwaysExecute = false
			fmt.Println(cSystem("IA> Modo auto-ejecución desactivado. Se pedirá confirmación."))
			fmt.Println()
			continue

		} else if input == "/help" {
			printHelp()
			continue

		} else if strings.HasPrefix(input, "/tiempo ") { // --- ¡NUEVO! ---
			prompt := strings.TrimPrefix(input, "/tiempo ")
			prompt = strings.TrimSpace(prompt)
			if prompt == "" {
				fmt.Println(cError("IA> Petición de tiempo vacía. Escribe /tiempo <lugar>."))
				fmt.Println()
				continue
			}
			handleWeatherCommand(client, selectedModel, prompt)

		} else if strings.HasPrefix(input, "/chat ") {
			prompt := strings.TrimPrefix(input, "/chat ")
			prompt = strings.TrimSpace(prompt)
			if prompt == "" {
				fmt.Println(cError("IA> Petición de chat vacía. Escribe /chat seguido de tu pregunta."))
				fmt.Println()
				continue
			}
			handleChatCommand(client, selectedModel, prompt)

		} else if strings.HasPrefix(input, "/") {
			prompt := strings.TrimPrefix(input, "/")
			prompt = strings.TrimSpace(prompt)
			if prompt == "" {
				fmt.Println(cError("IA> Petición de IA vacía. Escribe / seguido de tu consulta."))
				fmt.Println()
				continue
			}
			if alwaysExecute {
				handleIACommandAuto(client, selectedModel, prompt)
			} else {
				if handleIACommandConfirm(client, state, selectedModel, prompt) {
					alwaysExecute = true
				}
			}

		} else {
			fmt.Println()
			cmd := exec.Command("bash", "-c", input)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			fmt.Println()
		}
	}

	fmt.Println(cSystem("\n¡Adiós!"))
}

// sanitizeIACommand (Sin cambios)
func sanitizeIACommand(rawCmd string) string {
	cmd := strings.TrimSpace(rawCmd)
	if strings.HasPrefix(cmd, "`") && strings.HasSuffix(cmd, "`") {
		cmd = strings.TrimPrefix(cmd, "`")
		cmd = strings.TrimSuffix(cmd, "`")
		return strings.TrimSpace(cmd)
	}
	if strings.HasPrefix(cmd, "```") && strings.HasSuffix(cmd, "```") {
		cmd = strings.TrimPrefix(cmd, "```")
		cmd = strings.TrimSuffix(cmd, "```")
		if strings.HasPrefix(cmd, "bash\n") {
			cmd = strings.TrimPrefix(cmd, "bash\n")
		} else if strings.HasPrefix(cmd, "sh\n") {
			cmd = strings.TrimPrefix(cmd, "sh\n")
		}
		return strings.TrimSpace(cmd)
	}
	return cmd
}

// --- ¡NUEVA FUNCIÓN DE TIEMPO! ---
// (Reemplaza a 'handleWebCommand')
func handleWeatherCommand(client *api.Client, modelName string, location string) {
	fmt.Println(cIA("IA> Consultando el tiempo...") + cSystem(" (Usando wttr.in)"))

	// 1. Preparar la URL de wttr.in (pide 1 día de previsión en formato JSON)
	// Usamos http:// para evitar problemas de certificados
	endpoint := fmt.Sprintf("http://wttr.in/%s?format=j1", url.QueryEscape(location))

	// 2. Realizar la petición HTTP
	httpClient := &http.Client{}
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		fmt.Println(cError(fmt.Sprintf("\nError al crear la petición web: %v", err)))
		return
	}
	// wttr.in requiere un User-Agent
	req.Header.Set("User-Agent", "terminal-ia-go-client")

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Println(cError(fmt.Sprintf("\nError al llamar a la API de wttr.in: %v", err)))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Println(cError(fmt.Sprintf("\nError de la API de wttr.in (código %d). ¿Localización correcta?", resp.StatusCode)))
		return
	}

	// 3. Leer y "Aumentar" (Preparar el contexto para Ollama)
	var wttrResp WttrResponse
	if err := json.NewDecoder(resp.Body).Decode(&wttrResp); err != nil {
		fmt.Println(cError(fmt.Sprintf("\nError al decodificar la respuesta de wttr.in: %v", err)))
		return
	}

	// 4. Comprobar si obtuvimos una respuesta
	if len(wttrResp.CurrentCondition) == 0 {
		fmt.Println(cError("\nLo siento, wttr.in no pudo encontrar esa localización."))
		fmt.Println()
		return
	}

	// 5. Construir el contexto
	current := wttrResp.CurrentCondition[0]
	desc := ""
	if len(current.WeatherDesc) > 0 {
		desc = current.WeatherDesc[0].Value
	}

	locName := location
	if len(wttrResp.NearestArea) > 0 && len(wttrResp.NearestArea[0].AreaName) > 0 {
		locName = wttrResp.NearestArea[0].AreaName[0].Value
		if len(wttrResp.NearestArea[0].Country) > 0 {
			locName += ", " + wttrResp.NearestArea[0].Country[0].Value
		}
	}


	contextSnippet := fmt.Sprintf(
		"Contexto del tiempo para %s:\nTemperatura: %s°C\nSensación térmica: %s°C\nDescripción: %s\n",
		locName,
		current.Temp_C,
		current.FeelsLikeC,
		desc,
	)

	// 6. Generar (Llamar a Ollama con el contexto)
	systemPrompt := "Eres un asistente de IA. Responde a la 'Pregunta del Usuario' en español, de forma concisa y amigable, basándote únicamente en el 'Contexto del tiempo' proporcionado."
	fullPrompt := fmt.Sprintf("%s\n\n%s\n\nPregunta del Usuario: ¿Qué tiempo hace en %s?", systemPrompt, contextSnippet, location)

	fmt.Println(cIA("IA> Generando respuesta..."))

	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	go func() {
		<-sigChan
		cancel()
	}()
	defer signal.Stop(sigChan)

	stream := true
	reqOllama := &api.GenerateRequest{
		Model:  modelName,
		Prompt: fullPrompt,
		Stream: &stream,
	}

	firstChunk := true
	streamHandler := func(r api.GenerateResponse) error {
		if firstChunk {
			fmt.Print("\r" + cIA("IA: ") + "    \r")
			firstChunk = false
		}
		fmt.Print(r.Response)
		return nil
	}

	err = client.Generate(ctx, reqOllama, streamHandler)
	if err != nil {
		if err == context.Canceled {
			fmt.Print(cError("\n[Stream cancelado]"))
		} else {
			fmt.Println(cError(fmt.Sprintf("\nError al generar respuesta de chat: %v", err)))
		}
	}
	fmt.Println()
}


// handleChatCommand (Sin cambios)
func handleChatCommand(client *api.Client, modelName string, userPrompt string) {
	systemPrompt := "Eres un asistente servicial, amigable y conversacional. Responde a las preguntas del usuario."
	fullPrompt := fmt.Sprintf("%s\n\nUsuario: %s", systemPrompt, userPrompt)

	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	go func() {
		<-sigChan
		cancel()
	}()

	defer signal.Stop(sigChan)

	fmt.Println(cIA("IA> Pensando...") + cSystem(" (Presiona Ctrl+C para cancelar)"))

	stream := true
	req := &api.GenerateRequest{
		Model:  modelName,
		Prompt: fullPrompt,
		Stream: &stream,
	}

	firstChunk := true

	streamHandler := func(r api.GenerateResponse) error {
		if firstChunk {
			fmt.Print("\r" + cIA("IA: ") + "    \r")
			firstChunk = false
		}
		fmt.Print(r.Response)
		return nil
	}

	err := client.Generate(ctx, req, streamHandler)

	if err != nil {
		if err == context.Canceled {
			fmt.Print(cError("\n[Stream cancelado]"))
		} else {
			fmt.Println(cError(fmt.Sprintf("\nError al generar respuesta de chat: %v", err)))
		}
	}

	fmt.Println()
}


// handleIACommandAuto (Sin cambios)
func handleIACommandAuto(client *api.Client, modelName string, userPrompt string) {
	systemPrompt := `Eres un experto en terminal de Linux y shell.
	Traduce la siguiente petición de lenguaje natural a un ÚNICO comando de shell.
	Responde SÓLO con el comando y nada más. No uses markdown, ni explicaciones.
	Petición: `
	fullPrompt := systemPrompt + userPrompt
	fmt.Println(cIA("IA> Procesando (auto)..."))

	req := &api.GenerateRequest{
		Model:  modelName,
		Prompt: fullPrompt,
		Stream: new(bool),
	}
	ctx := context.Background()
	var resp api.GenerateResponse
	responseHandler := func(r api.GenerateResponse) error {
		resp = r
		return nil
	}
	if err := client.Generate(ctx, req, responseHandler); err != nil {
		fmt.Fprintln(os.Stderr, cError(fmt.Sprintf("Error al contactar con Ollama: %v", err)))
		return
	}

	comandoSugerido := sanitizeIACommand(resp.Response)

	fmt.Println()
	fmt.Println(cSystem("ejecutando (auto):"))
	fmt.Println(comandoSugerido)
	fmt.Println()

	cmd := exec.Command("bash", "-c", comandoSugerido)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, cError("IA> El comando falló."))
	}
	fmt.Println()
}

// handleIACommandConfirm (Sin cambios)
func handleIACommandConfirm(client *api.Client, state *liner.State, modelName string, userPrompt string) bool {
	systemPrompt := `Eres un experto en terminal de Linux y shell.
	Traduce la siguiente petición de lenguaje natural a un ÚNICO comando de shell.
	Responde SÓLO con el comando y nada más. No uses markdown, ni explicaciones.
	Petición: `
	fullPrompt := systemPrompt + userPrompt
	fmt.Println(cIA("IA> Procesando..."))

	req := &api.GenerateRequest{
		Model:  modelName,
		Prompt: fullPrompt,
		Stream: new(bool),
	}
	ctx := context.Background()
	var resp api.GenerateResponse
	responseHandler := func(r api.GenerateResponse) error {
		resp = r
		return nil
	}
	if err := client.Generate(ctx, req, responseHandler); err != nil {
		fmt.Fprintln(os.Stderr, cError(fmt.Sprintf("Error al contactar con Ollama: %v", err)))
		return false
	}

	comandoSugerido := sanitizeIACommand(resp.Response)

	fmt.Println(cSystem("---"))
	fmt.Println(cIA("IA> Comando sugerido:"))
	fmt.Printf("\n%s\n\n", comandoSugerido)
	fmt.Println(cSystem("---"))

	prompt := "IA> ¿Ejecutar? [s/N/X (Siempre)]: "
	confirmacion, err := state.Prompt(prompt)

	if err != nil {
		if err == io.EOF || err == liner.ErrPromptAborted {
			fmt.Println(cSystem("\nCancelado."))
			return false
		}
		fmt.Println(cError(fmt.Sprintf("Error al leer la confirmación: %v", err)))
		return false
	}

	state.AppendHistory(confirmacion)

	confirmacion = strings.TrimSpace(strings.ToLower(confirmacion))

	switch confirmacion {
		case "s":
			fmt.Println(cSystem("IA> Ejecutando..."))
			fmt.Println()
			fmt.Println(cSystem("ejecutando:"))
			fmt.Println(comandoSugerido)
			fmt.Println()
			cmd := exec.Command("bash", "-c", comandoSugerido)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, cError("IA> El comando falló."))
			}
			fmt.Println()
			return false
		case "x":
			fmt.Println(cSystem("IA> Ejecutando y activando modo 'auto'..."))
			fmt.Println()
			fmt.Println(cSystem("ejecutando:"))
			fmt.Println(comandoSugerido)
			fmt.Println()
			cmd := exec.Command("bash", "-c", comandoSugerido)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, cError("IA> El comando falló."))
			}
			fmt.Println()
			fmt.Println(cSystem("IA> Modo auto-ejecución activado. Escribe '/ask' para desactivarlo."))
			fmt.Println()
			return true
		default:
			fmt.Println(cSystem("IA> Cancelado."))
			fmt.Println()
			return false
	}
}
